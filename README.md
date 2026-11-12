# Unofficial field-test build

evcc 0.319.0 with the Policharger charger driver, built from branch `policharger-0.319.0` of this fork. For field testing only. Not an evcc release.

Built with Go 1.27.0, `CGO_ENABLED=0`, `-tags=release`. The uncompressed binary's SHA-256 starts with `7aa4ad15a4a7355f`. Version 0.319.0-policharger.2: status is read-only, a heartbeat keeps the charger armed.

The previous 0.318.1 build is in this branch's history.
