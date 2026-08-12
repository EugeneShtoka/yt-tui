# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report privately via GitHub's
[Private vulnerability reporting](https://github.com/EugeneShtoka/yt-tui/security/advisories/new)
(the **Security → Report a vulnerability** button on the repository). This keeps
the report confidential until a fix is available.

Please include, as far as you can:

- the affected component (`yt-tui` client, `yt-tuid` daemon, or both) and version
  (`yt-tui --version`),
- a description of the issue and its impact,
- steps to reproduce or a proof of concept,
- any suggested remediation.

You can expect an initial acknowledgement within a few days. Once a fix is
released, we're happy to credit reporters who wish to be named.

## Scope and things to keep in mind

yt-tui is a local, single-user tool, but a few areas are security-relevant and
worth flagging if you find a problem:

- **The daemon (`yt-tuid`)** authenticates every request with a single shared
  **bearer token** and is intended to run behind **TLS**. It is single-tenant by
  design — it is *not* built for multi-user hosting. Report anything that lets a
  request bypass the token check, or that leaks the token.
- **Browser cookies.** The app reads your browser's YouTube cookies via yt-dlp
  for personalized feeds. Report any path that could exfiltrate cookies or write
  them somewhere unexpected.
- **Subprocess execution.** yt-dlp and the media player are launched as child
  processes. Report any way untrusted input (e.g. a video title or URL) could
  lead to command injection or unintended argument evaluation.
- **TLS material / tokens in logs.** Report any case where secrets end up in the
  debug log or on stderr.

## Supported versions

This is a pre-1.0 project; only the latest release is supported. Please upgrade
to the newest release before reporting an issue.
