# Public failure scenarios

Reviewed 2026-10-22 Pacific. These are historical reporter claims, not reproduced
OrbWin defects, current cross-product comparisons, or proof of a shared root cause.
No destructive steps from these reports were run on this host.

| ID | Primary source and reported environment | Failure signature | Qualification scenario / reproduction status |
| --- | --- | --- | --- |
| R-001 | [Microsoft WSL #13578](https://github.com/microsoft/WSL/issues/13578), filed Oct 8 2025: Windows 11 Home build 26100.6584, WSL 2.5.10.0, kernel 6.6.87.2-1, Ubuntu 24.04.2, Docker Desktop 4.37.1 | Reporter describes forced Windows shutdown followed by intermittent `HCS_E_CONNECTION_TIMEOUT` and failed WSL integration | Disposable-host interrupted-workload/reboot test; classify WSL/platform failure separately from UI/engine failure. Not reproduced. |
| R-002 | [Testcontainers Java #4395](https://github.com/testcontainers/testcontainers-java/issues/4395), filed Aug 25 2021: Testcontainers 1.16.0 versus 1.15.3, Docker Desktop 3.6.0, macOS 10.14.6, tests inside a devcontainer | Helper container connection failure at an inferred bridge address during Compose startup | Test host/WSL/devcontainer client discovery, cleanup helper connectivity and socket-mount targeting. Windows is a proposed portability scenario, not the reported reproducer. Not reproduced. |
| R-003 | [Docker for Windows #14198](https://github.com/docker/for-win/issues/14198), filed Jul 18 2024: labels 4.32.0/4.34.1, body CLI 27.0.3, Compose 2.28.1 | Unexpectedly stopped Ubuntu integration and HTTP 500 from version/info; restart/skip did not resolve the reporter's issue | Inject an owned integration bridge failure and preserve diagnostics; distinguish client HTTP error from daemon crash. Not reproduced. |

Do not import reporter reset/shutdown suggestions into OrbWin's recovery policy.
Add exact tested versions, raw results and failure ownership when a scenario is
executed. The user's crash anecdote has no established match to these reports.
