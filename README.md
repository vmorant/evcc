# Unofficial field-test build

evcc 0.319.0 with the Policharger charger driver, built from branch `policharger-0.319.0` of this fork. For field testing only. Not an evcc release.

Built with Go 1.27.0, `CGO_ENABLED=0`, `-tags=release`. The uncompressed binary's SHA-256 starts with `8b726aa030272e63`. Version 0.319.0-policharger.3: same driver as pull request evcc-io/evcc#36129 at commit 14e65a0.

The previous 0.318.1 build is in this branch's history.

`evcc_0.319.0-policharger-slim_linux-amd64.gz`: version 0.319.0-policharger.slim, the simplified driver of pull request evcc-io/evcc#36129 at commit 820ea59 (branch `policharger-0.319.0-slim`). Uncompressed SHA-256 starts with `120d2ed58448476a`.
