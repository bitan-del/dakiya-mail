// Opt-out hints read by 1Password, LastPass, Bitwarden and Dashlane respectively.
// Spread onto inputs holding someone else's credentials (a mailbox's SMTP/IMAP login).
export const PASSWORD_MANAGER_IGNORE = {
    "data-1p-ignore": "true",
    "data-lpignore": "true",
    "data-bwignore": "true",
    "data-form-type": "other",
} as const;
