# Deploying PLMS

Everything except Kafka deploys to **Render in one pass** from `render.yaml` (a Render "Blueprint").
Render does not offer Kafka, so Kafka comes from **Aiven's free plan**. The website can stay on
Render, or move to **Vercel** (Option B at the end).

```
 Browser ──► plms-web (Render static site or Vercel)
                │  /api/*  is forwarded to ↓   (the browser only sees one website)
                ▼
             plms-api (Render, Docker) ──► plms-db (Render Postgres)
                │  outbox relay + email sender run inside the API (RUN_NOTIFIER=true)
                ▼
             Kafka (Aiven)          Email (Resend, optional)
```

## What the free plans give you

| Part | Where | Free plan limits |
| --- | --- | --- |
| Database | Render Postgres | 1 GB; **deleted 30 days after creation** unless upgraded (14-day grace) |
| API + email sender | Render web service | Sleeps after 15 min without visitors; the next visit takes about a minute to wake it |
| Website | Render static site (or Vercel) | Free |
| Kafka | Aiven | 5 topics × 2 partitions; powers off when idle (turn it on again in the Aiven console) |
| Email | Resend | 100 emails/day; needs your own verified domain to email anyone but yourself |

While the API sleeps, nothing is lost: ticket events wait in the database and are sent when it
wakes up. That's fine for a demo. For a real car park, use paid plans (see the end of this file).

---

## Step 0 — Put the code on GitHub

Render and Vercel deploy from a Git repository. The project is already connected to
`https://github.com/aamita96/parking-lot-management-system`, so commit and push the deployment
files (`render.yaml`, `DEPLOY.md`, `frontend/vercel.json` and the backend changes):

```powershell
cd "D:\MSI_Backup\Workspace\Parking Lot System"
git status            # check that .env and node_modules are NOT listed
git add .
git commit -m "Add Render/Vercel deployment setup"
git push
```

In the steps below, "your repository" means this one.

## Step 1 — Create the Kafka service on Aiven (about 5 minutes)

1. Sign up at https://aiven.io (no credit card needed).
2. **Create service → Apache Kafka → Free plan**. Pick a region close to Singapore if one is offered.
   Name it `plms-kafka`. Wait until its status is **Running**.
3. Open the service → **Overview → Connection information**. Aiven shows **Host**, **Port**,
   **User**, **Password** and a **CA Certificate**. This is the SASL login PLMS uses
   (SCRAM-SHA-256 over TLS):
   - **Service URI** (`host:port`) goes into `KAFKA_BROKERS`. For this project it is already
     in `render.yaml`: `kafka-26e67ac0-parking-lot-management-system.d.aivencloud.com:20023`.
   - **User** goes into `KAFKA_USERNAME` (usually `avnadmin`).
   - **Password** goes into `KAFKA_PASSWORD`. Keep it secret; never commit it to Git.
   - Download the **CA Certificate** (`ca.pem`). You'll upload it to Render in step 2.
4. Open **Topics → Add topic** and create:
   - `parking.ticket-events.v1` with **2** partitions
   - `parking.ticket-events.v1.dlq` with **1** partition

   (The API also tries to create them. Doing it yourself avoids permission problems.)

## Step 2 — Deploy everything else on Render (one Blueprint)

1. Sign up at https://render.com with your GitHub account.
2. **New → Blueprint** → select your repository. Render reads `render.yaml` and lists
   `plms-db`, `plms-api` and `plms-web`.
3. Render asks for the values marked `sync: false`:

   | Variable | What to enter |
   | --- | --- |
   | `ADMIN_BOOTSTRAP_EMAIL` | Your admin login email |
   | `ADMIN_BOOTSTRAP_PASSWORD` | A strong password (this creates the first admin) |
   | `KAFKA_USERNAME` | Aiven **User** (usually `avnadmin`) |
   | `KAFKA_PASSWORD` | Aiven **Password** |
   | `RESEND_API_KEY` | Leave empty for now |

   `KAFKA_BROKERS`, `KAFKA_TLS=true` and `KAFKA_SASL_MECHANISM=SCRAM-SHA-256` are already set in
   `render.yaml`.

   **Already created the Blueprint earlier?** Render only asks for these values the first time.
   Add `KAFKA_USERNAME` and `KAFKA_PASSWORD` yourself under **plms-api → Environment →
   Environment Variables**.
4. Click **Apply**. The first API build takes about 5–10 minutes.
5. Upload the CA certificate: open **plms-api → Environment → Secret Files → Add Secret File**,
   name it exactly `ca.pem`, and paste the full contents of the downloaded CA certificate
   (including the `-----BEGIN CERTIFICATE-----` and `-----END CERTIFICATE-----` lines).
   Render puts it at `/etc/secrets/ca.pem`, which is where `KAFKA_CA_CERT` points. Save; Render
   redeploys the API.
6. **Check the two URLs.** On the dashboard, note the real addresses of `plms-api` and `plms-web`.
   If Render added a suffix (because the name was taken), e.g. `https://plms-api-ab12.onrender.com`,
   update the three lines marked `← check` in `render.yaml`, then commit and push. Render applies
   Blueprint changes automatically.

## Step 3 — Check that it works

1. Open `https://<plms-api URL>/healthz`. It should show `{"status":"ok"}`. (The first visit can take a minute while it wakes up.)
2. Open **plms-api → Logs**. You should see `api listening`, `notifier running` and `kafka topics ready`.
3. Open the **plms-web** URL:
   - The availability page shows the three panels.
   - **Register** a vehicle. In demo mode the 6-digit code is shown on screen.
   - Go to `/admin`, sign in with the admin email and password from step 2, open the **Simulator** and send a vehicle.
     The event panel should show **On Kafka** and **Email sent** within a few seconds.

---

## Option B — Website on Vercel instead of Render

The API and database stay on Render (steps 0–2). Only the website moves.

1. In `frontend/vercel.json`, set the `destination` to your real plms-api URL. Commit and push.
2. On https://vercel.com: **Add New → Project** → import your repository, then set:
   - **Root Directory**: `frontend`
   - **Framework Preset**: Vite (build command `npm run build`, output directory `dist`)
   - **Node.js version** (Project Settings → General): 22 or newer
3. Deploy, and note the Vercel URL (e.g. `https://plms.vercel.app`).
4. On Render, open **plms-api → Environment** and set `PUBLIC_URL` and `CORS_ORIGINS` to the Vercel URL.
5. Delete the `plms-web` static site on Render (or remove it from `render.yaml`) so you don't have two websites.

**Why `/api` is forwarded instead of calling the API directly:** the admin login uses a secure
cookie that browsers only send to the same website. Forwarding `/api/*` through the website keeps
everything on one address, so login works and no extra CORS setup is needed. This is the same thing
nginx does in the local Docker setup.

---

## Sending real email (Resend)

1. Create an account at https://resend.com and **verify a domain** you own (add the DNS records it shows).
2. Create an API key.
3. On **plms-api → Environment**, set:
   - `EMAIL_ENABLED=true`
   - `RESEND_API_KEY=<key>`
   - `EMAIL_FROM=PLMS <no-reply@your-domain>`
   - `DEV_EXPOSE_CODES=false` (stops showing codes on screen)

Until then, the on-screen demo codes mean anyone can register with any email address. That's fine
for a demo, but not for real use.

## Before using it for a real car park

- Upgrade **plms-db** to a paid plan (the free one is deleted after 30 days) and **plms-api** to a
  paid plan (so it doesn't sleep).
- Set `APP_ENV=production` and `SIMULATOR_ENABLED=false` (the API refuses to start with the
  simulator on in production).
- Optional: run the email sender separately. Create a Render **Background Worker** from the same
  repo (Docker, `./backend/Dockerfile`, context `./backend`), set its **Docker Command** to
  `/usr/local/bin/notifier`, give it the same environment variables, and set `RUN_NOTIFIER=false`
  on plms-api.

## Troubleshooting

| What you see | What to check |
| --- | --- |
| API log: `Kafka settings are invalid` | The Secret File must be named exactly `ca.pem`, with the whole certificate pasted in |
| API log mentions `SASL` or `authentication failed` | `KAFKA_USERNAME` / `KAFKA_PASSWORD` must match Aiven's User and Password |
| API log keeps saying `kafka topic not ready yet` | Is the Aiven service **Running** (it powers off when idle)? Did you create the two topics? Is `KAFKA_BROKERS` the host:port Aiven shows next to the User and Password? |
| Website shows "Can't reach the server" | The `/api/*` destination URL in `render.yaml` / `vercel.json` |
| First page load is very slow or fails once | The free API was asleep; wait a minute and reload |
| Event panel stuck on **In outbox** | Kafka isn't reachable; the gate still works and events are sent once Kafka is back |
| Admin keeps getting sent back to the login page | Make sure you open the **website** URL, not the API URL |
