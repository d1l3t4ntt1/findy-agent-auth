package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/duo-labs/webauthn.io/session"
	"github.com/duo-labs/webauthn/protocol"
	"github.com/duo-labs/webauthn/webauthn"
	"github.com/findy-network/findy-agent-auth/enclave"
	"github.com/findy-network/findy-common-go/jwt"
	"github.com/findy-network/findy-common-go/utils"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	. "github.com/lainio/err2"
	"github.com/rs/cors"
)

const defaultPort = 8080

var (
	loggingFlags   string
	port           int = defaultPort
	agencyAddr     string
	agencyPort     int
	rpID           string
	rpOrigin       string
	jwtSecret      string
	webAuthn       *webauthn.WebAuthn
	sessionStore   *session.Store
	enclaveFile    = "fido-enclave.bolt"
	enclaveBackup  = ""
	enclaveKey     = "15308490f1e4026284594dd08d31291bc8ef2aeac730d0daf6ff87bb92d4336c"
	backupInterval = 24 // hours
	findyAdmin     = "findy-root"
	certPath       = "./cert"
	isHTTPS        = true

	startServerCmd = flag.NewFlagSet("server", flag.ExitOnError)

	defaultOrigin = fmt.Sprintf("http://localhost:%d", port)
)

type AccessToken struct {
	Token string `json:"token"`
}

func init() {
	startServerCmd.StringVar(&loggingFlags, "logging", "-logtostderr=true -v=2", "logging startup arguments")
	startServerCmd.IntVar(&port, "port", defaultPort, "server port")
	startServerCmd.StringVar(&agencyAddr, "agency", "guest", "agency gRPC server addr")
	startServerCmd.IntVar(&agencyPort, "gport", 50051, "agency gRPC server port")
	startServerCmd.StringVar(&rpID, "domain", "localhost", "the site domain name")
	startServerCmd.StringVar(&rpOrigin, "origin", defaultOrigin, "origin URL for Webauthn requests")
	startServerCmd.StringVar(&jwtSecret, "jwt-secret", "", "secure key for JWT token generation")
	startServerCmd.StringVar(&enclaveFile, "sec-file", enclaveFile, "secure enclave DB file name")
	startServerCmd.StringVar(&enclaveBackup, "sec-backup-file", enclaveBackup, "secure enclave DB backup base file name")
	startServerCmd.StringVar(&enclaveKey, "sec-key", enclaveKey, "sec-enc master key, SHA-256, 32-byte hex coded")
	startServerCmd.IntVar(&backupInterval, "sec-backup-interval", backupInterval, "secure enclave backup interval in hours")
	startServerCmd.StringVar(&findyAdmin, "admin", findyAdmin, "admin ID used for this agency ecosystem")
	startServerCmd.StringVar(&certPath, "cert-path", certPath, "cert root path where server and client certificates exist")
}

func main() {
	defer CatchTrace(func(_ error) {
		glog.Warningln("")
	})
	Check(startServerCmd.Parse(os.Args[1:]))
	utils.ParseLoggingArgs(loggingFlags)

	u := URL.Try(url.Parse(rpOrigin))
	isHTTPS = u.Scheme == "https"

	glog.V(3).Infoln(
		"\nlogging:", loggingFlags,
		"\norigin host:", u.Host,
		"\nlisten port:", port,
		"\norigin port:", u.Port(),
		"\nHTTPS ==", isHTTPS,
	)

	Check(enclave.InitSealedBox(enclaveFile, enclaveBackup, enclaveKey))
	enclave.Init(certPath, agencyAddr, agencyPort)

	if jwtSecret != "" {
		jwt.SetJWTSecret(jwtSecret)
	}

	var err error
	webAuthn, err = webauthn.New(&webauthn.Config{
		RPDisplayName: "OP Lab Corp.", // Display Name for your site
		RPID:          rpID,           // Generally the domain name for your site
		RPOrigin:      rpOrigin,       // The origin URL for WebAuthn requests
		// RPIcon: "https://duo.com/logo.png", // Optional icon URL for your site
	})
	Check(err)
	sessionStore, err = session.NewStore()
	Check(err)

	r := mux.NewRouter()

	r.HandleFunc("/register/begin/{username}", BeginRegistration).Methods("GET")
	r.HandleFunc("/register/finish/{username}", FinishRegistration).Methods("POST")
	r.HandleFunc("/login/begin/{username}", BeginLogin).Methods("GET")
	r.HandleFunc("/login/finish/{username}", FinishLogin).Methods("POST")

	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./")))

	// TODO: figure out CORS policy
	hCors := cors.New(cors.Options{
		AllowedOrigins:   []string{rpOrigin},
		AllowCredentials: true,
		// Enable Debugging for testing, consider disabling in production
		Debug: true,
	})

	backupTickerDone := enclave.BackupTicker(time.Duration(backupInterval) * time.Hour)

	serverAddress := fmt.Sprintf(":%d", port)
	if glog.V(1) {
		glog.Infoln("starting server at", serverAddress)
	}
	if isHTTPS {
		certFile := filepath.Join(certPath, "server.crt")
		keyFile := filepath.Join(certPath, "server.key")
		glog.V(3).Infoln("starting TLS server with:\n",
			certFile, "\n", keyFile)
		err = http.ListenAndServeTLS(serverAddress, certFile, keyFile,
			hCors.Handler(r))
	} else {
		err = http.ListenAndServe(serverAddress, hCors.Handler(r))
	}
	if err != nil {
		glog.Infoln("listen error:", err)
	}
	backupTickerDone <- struct{}{}
}

func BeginRegistration(w http.ResponseWriter, r *http.Request) {
	defer Catch(func(err error) {
		glog.Warningln("begin registration error:", err)
	})

	// get username/friendly name
	vars := mux.Vars(r)
	username, ok := vars["username"]
	glog.V(1).Infoln("begin registration", username)
	if !ok {
		jsonResponse(w, fmt.Errorf("must supply a valid username i.e. foo@bar.com"), http.StatusBadRequest)
		return
	}

	var err error

	// get user
	user, exists, err := enclave.GetUser(username)
	Check(err)

	// TODO: add functionality for registering new device
	if exists {
		jsonResponse(w, fmt.Errorf("must supply a valid username i.e. foo@bar.com"), http.StatusBadRequest)
		return
	}

	// user doesn't exist, create new user
	displayName := strings.Split(username, "@")[0]
	glog.V(2).Infoln("adding new user:", displayName)
	user = enclave.NewUser(username, displayName)
	Check(enclave.PutUser(user))

	registerOptions := func(credCreationOpts *protocol.PublicKeyCredentialCreationOptions) {
		credCreationOpts.CredentialExcludeList = user.CredentialExcludeList()
		glog.V(1).Infoln("credexcl:", len(credCreationOpts.CredentialExcludeList))
	}

	defer Handle(&err, func() {
		glog.Errorln("error:", err)
		jsonResponse(w, err.Error(), http.StatusInternalServerError)
	})

	glog.V(1).Infoln("begin registration to webAuthn")
	options, sessionData, err := webAuthn.BeginRegistration(
		user,
		registerOptions,
	)
	Check(err)
	glog.V(1).Infof("sessionData: %v", sessionData)

	// store session data as marshaled JSON
	glog.V(1).Infoln("store session data")
	Check(sessionStore.SaveWebauthnSession("registration", sessionData, r, w))

	jsonResponse(w, options, http.StatusOK)
	glog.V(1).Infoln("begin registration end", username)
}

func FinishRegistration(w http.ResponseWriter, r *http.Request) {
	defer Catch(func(err error) {
		glog.Warningln("BEGIN finish registration:", err)
	})

	var err error

	// get username
	vars := mux.Vars(r)
	username := vars["username"]
	glog.V(1).Infoln("finish registration", username)

	defer Handle(&err, func() {
		glog.Errorln("error:", err)
		jsonResponse(w, err.Error(), http.StatusInternalServerError)
	})

	glog.V(1).Infoln("getting existing user", username)
	user, err := enclave.GetExistingUser(username)
	Check(err)

	glog.V(1).Infoln("get session data for registration")
	sessionData, err := sessionStore.GetWebauthnSession("registration", r)
	Check(err)

	glog.V(1).Infoln("call web authn finish registration and getting credential")
	credential, err := webAuthn.FinishRegistration(user, sessionData, r)
	Check(err)

	// Add needed data to User
	user.AddCredential(*credential)
	Check(user.AllocateCloudAgent(findyAdmin))
	// Persist that data
	Check(enclave.PutUser(user))

	jsonResponse(w, "Registration Success", http.StatusOK)
	glog.V(1).Infoln("END finish registration", username)
}

func BeginLogin(w http.ResponseWriter, r *http.Request) {
	defer Catch(func(err error) {
		glog.Warningln("begin login", err)
	})

	var err error

	// get username
	vars := mux.Vars(r)
	username := vars["username"]
	glog.V(1).Infoln("BEGIN begin login", username)

	defer Handle(&err, func() {
		jsonResponse(w, err.Error(), http.StatusInternalServerError)
	})

	// get user
	user, err := enclave.GetExistingUser(username)
	Check(err)

	// generate PublicKeyCredentialRequestOptions, session data
	options, sessionData, err := webAuthn.BeginLogin(user)
	Check(err)

	// store session data as marshaled JSON
	err = sessionStore.SaveWebauthnSession("authentication", sessionData, r, w)
	Check(err)

	jsonResponse(w, options, http.StatusOK)
	glog.V(1).Infoln("END begin login", username)
}

func FinishLogin(w http.ResponseWriter, r *http.Request) {
	defer Catch(func(err error) {
		glog.Warningln("finish login error:", err)
	})

	var err error

	// get username
	vars := mux.Vars(r)
	username := vars["username"]
	glog.V(1).Infoln("BEGIN finish login:", username)

	defer Handle(&err, func() {
		jsonResponse(w, err.Error(), http.StatusInternalServerError)
	})

	// get user
	user, err := enclave.GetExistingUser(username)
	Check(err)

	// load the session data
	sessionData, err := sessionStore.GetWebauthnSession("authentication", r)
	Check(err)

	// in an actual implementation, we should perform additional checks on
	// the returned 'credential', i.e. check 'credential.Authenticator.CloneWarning'
	// and then increment the credentials counter
	_, err = webAuthn.FinishLogin(user, sessionData, r)
	Check(err)

	// handle successful login
	jsonResponse(w, &AccessToken{Token: user.JWT()}, http.StatusOK)
	glog.V(1).Infoln("END finish login", username)
}

// from: https://github.com/duo-labs/webauthn.io/blob/3f03b482d21476f6b9fb82b2bf1458ff61a61d41/server/response.go#L15
func jsonResponse(w http.ResponseWriter, d interface{}, c int) {
	dj, err := json.Marshal(d)
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(c)
	glog.V(1).Infof("reply json:\n%s", dj)
	fmt.Fprintf(w, "%s", dj)
}
