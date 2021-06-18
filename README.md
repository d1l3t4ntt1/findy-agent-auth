# findy-agent-auth

[![test](https://github.com/findy-network/findy-agent-auth/actions/workflows/test.yml/badge.svg)](https://github.com/findy-network/findy-agent-auth/actions/workflows/test.yml)

Authentication services for Findy agency.

## Server

This project provides FIDO2/WebAuthn authentication service for findy agency clients. The service implements the WebAuthn protocol providing means to securely
* initiate user registration,
* finish user registration,
* initiate authentication and
* finish authentication.

The authentication service can be utilized for example by any web app running in [a compatible browser](https://caniuse.com/?search=webauthn).

During a successful registration the user is onboarded to [findy core agency](https://github.com/findy-network/findy-agent) and an Aries compatible cloud agent is allocated for the user. After registration, user can generate a token for findy agency with this authentication service. This token is required by [agency API](https://github.com/findy-network/findy-agent-api).

### Usage

```sh
$ go run . \
    --port 8088 \                       # port for this service
    --origin http://localhost:3000 \    # origin for browser requests
    --cors=true \                       # use CORS headers
    --agency localhost \                # core agency GRPC server address
    --gport 50051 \                     # core agency GRPC server port
    --cert-path /path/to/agency/cert \  # path to agency GRPC cert
    --jwt-secret agency-jwt-secret \    # agency JWT secret
    --admin agency-admin-id             # agency admin ID
```

## Client

This project provides also library for authenticating headless clients. Headless authenticator is needed when implementing (organisational) services needing cloud agents. Check [agency CLI](https://github.com/findy-network/findy-agent-cli) for reference implementation.