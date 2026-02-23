---
id: push-to-gitlab
type: how-to
status: stable
tags: [how-to, git, gitlab, setup]
---

# Push this repo to GitLab

Goal: push the plexTuner repo to a GitLab project (create project if needed, then push).

Preconditions
-------------
- Repo cloned and on `main` with at least one commit.
- Remote is set to `https://gitlab.home/keith/plexTuner.git` (or you will set it).

Steps
-----

### 1. Create the project on GitLab

1. Open **https://gitlab.home/keith** (or your group/user).
2. Click **New project** → **Create blank project**.
3. **Project name:** `plexTuner`.
4. **Visibility:** Private (or your choice).
5. **Do not** initialize with a README (we already have one).
6. Click **Create project**.

### 2. Push from this machine

From the repo root:

**If using HTTPS** (and your GitLab uses a self-signed certificate):

```bash
GIT_SSL_NO_VERIFY=1 git push -u origin main
```

You'll be prompted for your GitLab username and password (or token).

**If using SSH** (after adding `gitlab.home` to `~/.ssh/known_hosts` and having a key in GitLab):

```bash
git remote set-url origin git@gitlab.home:keith/plexTuner.git
git push -u origin main
```

### 3. Optional: Create project via API (with token)

If you have a GitLab personal access token (with `api` scope):

```bash
export GITLAB_TOKEN="your-token"
curl --request POST \
  --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  --header "Content-Type: application/json" \
  --data '{"name":"plexTuner","visibility":"private"}' \
  "https://gitlab.home/api/v4/projects"
```

Then run the push step above.

Verify
------
- `git remote -v` shows your GitLab remote.
- `git push` succeeds; GitLab project shows the same commits.

Rollback
--------
- Remove remote: `git remote remove origin`. Re-add when ready.

Troubleshooting
---------------
- **SSL errors (HTTPS):** Use `GIT_SSL_NO_VERIFY=1` only for self-signed; prefer fixing CA or using SSH.
- **Permission denied (SSH):** Ensure your key is added to GitLab and `ssh -T git@gitlab.home` works.

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
