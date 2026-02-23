# Push this repo to GitLab

The repo is initialized with `main` and one commit. Remote is set to:

**https://gitlab.home/keith/plexTuner.git**

## 1. Create the project on GitLab

1. Open **https://gitlab.home/keith** (or your group/user).
2. Click **New project** → **Create blank project**.
3. **Project name:** `plexTuner`.
4. **Visibility:** Private (or your choice).
5. **Do not** initialize with a README (we already have one).
6. Click **Create project**.

## 2. Push from this machine

From the repo root (`/home/keith/Documents/code/plexTuner`):

**If using HTTPS** (and your GitLab uses a self-signed certificate):

```bash
GIT_SSL_NO_VERIFY=1 git push -u origin main
```

You’ll be prompted for your GitLab username and password (or token).

**If using SSH** (after adding `gitlab.home` to `~/.ssh/known_hosts` and having a key in GitLab):

```bash
git remote set-url origin git@gitlab.home:keith/plexTuner.git
git push -u origin main
```

## Optional: Create project via API (with token)

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
