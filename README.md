# findy-agent-auth

Authentication services for Findy agency

Includes WebAuthn sample server. That will work as a reference implementation how to allocate `findy-agent` cloud agents from fido2 compatible web wallets.

A current version of the WebAuthn server can be started from package root:

```shell script
$ go run .
```

## Publishing new version

Release script will tag the current version and push the tag to remote. This will trigger e2e-tests in CI automatically and if they succeed, the tag is merged to master.

Release script assumes it is triggered from dev branch. It takes one parameter, the next working version. E.g. if current working version is 0.1.0, following will release version 0.1.0 and update working version to 0.2.0.

```bash
git checkout dev
./release 0.2.0
```

Implement e2e test to release workflow according to your project needs.