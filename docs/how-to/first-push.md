---
id: first-push
type: how-to
status: stable
tags: [how-to, git, first-push, remote]
---

# First push (add remote and push)

Goal: push this repo to a remote (GitHub, GitLab, or self-hosted) after creating the project there.

Preconditions
-------------
- Repo cloned or created from template, on `main` with at least one commit.
- You have created an **empty** project on your Git host (no README init).

Steps
-----

### 1. Create the project on your host

- **GitHub:** New repository → no README, no .gitignore.
- **GitLab:** New project → Create blank project, do not initialize.
- **Self-hosted:** Same idea: empty project, no initial commit.

### 2. Add remote and push

From the repo root, set the remote to your project URL and push:

**HTTPS** (replace with your host and path):

```bash
git remote add origin https://YOUR_HOST/YOUR_ORG/your-repo.git
git push -u origin main
```

For self-signed HTTPS: `GIT_SSL_NO_VERIFY=1 git push -u origin main` (use only when you control the host).

**SSH** (replace with your host and path):

```bash
git remote add origin git@YOUR_HOST:YOUR_ORG/your-repo.git
git push -u origin main
```

Verify
------
- `git remote -v` shows your remote.
- Push succeeds; the host shows your commits.

See also
--------
- [Docs index](../index.md)
- [How-to index](index.md)

Related ADRs
------------
- *(none)*

Related runbooks
----------------
- *(none)*
