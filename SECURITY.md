# Security Policy

## Reporting a Vulnerability

We take the security of Email AI Agent seriously. If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public issue for security vulnerabilities.
2. Email the lead maintainer directly at `cesare@disroot.ort`.
3. Include the following in your report:
   - A description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if available)

## Response Timeline

- We aim to respond within 72 hours.
- Vulnerabilities will be patched in a timely manner.
- Credit will be given to reporters (unless anonymity is requested).

## Security Best Practices

When running this application:

- Store all secrets in `.env` files or secure secret management systems.
- Never commit `.env` files or expose credentials in logs.
- Run with minimal required permissions.
- Keep the SQLite state file (`state/emailer.db`) with mode 0600.
- Regularly update dependencies to receive security patches.

## Known Security Considerations

This application handles:
- IMAP credentials (stored/configured, never logged)
- LLM API keys (sent via Authorization header, never in URLs)
- Email content (processed for classification, not stored)
