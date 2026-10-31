# SyndiRelay third-party dependencies and platform authorization

SyndiRelay original code is MIT; copied Inkerio PlantUML helper code is also from the MIT engine. Third-party components (Go modules, micromark, Sharp, Chromium and diagram tooling if installed) retain their own licenses. The repository MIT LICENSE does **not** relicense upstream modules, browser assets or user-authored articles.

Historical BlogCTL binaries through v0.1.193 included the `tls-client` / `fhttp` transport; the `fhttp` license could not be verified for the complete old distribution. The new source and Go module exclude those dependencies; source cleanup does not retrospectively repair old release ZIPs or establish third-party API authorization.

The private migration repo is not yet approved for public releases. Package creation requires complete, current dependency license texts for every GOOS/GOARCH target and a review of actual npm runtime modules; `cmd/maint package` fails closed if its verified license bundle is missing. Runtime Node dependencies are installed by the user with `npm ci`, not silently vendored in this source repository.

Some platform publisher implementations interact with authenticated browser sessions and may use private or undocumented endpoints. That is a separate contractual/authorization issue from open-source licensing. Keep integrations unadvertised and do not publish or automatically run them until platform-by-platform review. A browser session or Cookie does not by itself grant authority to automate an endpoint.

Never include users' content, session Cookies, access tokens, configuration secrets or local publishing databases in the repository or any downloadable package.
