# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's private vulnerability reporting: open the repository's **Security** tab and choose **Report a vulnerability**. Do not open a public issue, pull request or discussion for a vulnerability.

Include what you found, how to reproduce it, the version (`mrimpact --version`) and the impact you expect. You will get an answer in the advisory; once a fix is released, the advisory is published with credit to you unless you prefer otherwise.

## Scope

Examples of what is in scope:

- the `mrimpact` binary, the GitHub Action (`action.yml`), the GitLab template (`ci/mrimpact.gitlab-ci.yml`) and `install.sh`;
- the handling of the forge token (`MRIMPACT_TOKEN`), for example sending it to another host;
- `install.sh` installing a binary whose checksum was not verified.

Running mrimpact on `pull_request_target` is unsupported and documented as unsafe ([reference](docs/reference.md#do-not-use-pull_request_target)).

## Supported versions

Security fixes go into the latest release.
