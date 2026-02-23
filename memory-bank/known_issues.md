# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Security

- **Credentials:** Secrets must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log secrets. Use `.env.example` as a template (no real values).

