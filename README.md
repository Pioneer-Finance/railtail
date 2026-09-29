# <img align="left" width="40" height="40" src="https://res.cloudinary.com/railway/image/upload/v1734036971/railtail_avdaue.png" alt="railtail logo"> railtail

railtail is a HTTP/TCP proxy for Railway workloads connecting to Tailscale
nodes. It listens on a local address and forwards traffic it receives on
the local address to a target Tailscale node address.

📣 This is a workaround until there are [full VMs available in Railway](https://help.railway.com/feedback/full-unix-v-ms-44eef294). Please upvote the thread if you want this feature!

## Usage

1. [Install and setup Tailscale](https://tailscale.com/kb/1017/install) on the
   machine you want to connect to. If you're using Tailscale as a subnet
   router, ensure you advertise the correct routes and approve the subnets
   in the Tailscale admin console.

2. Deploy this template to Railway:

   [![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/template/railtail?referralCode=EPXG5z)

3. In services that need to connect to the Tailscale node, connect to your
   railtail service using the `RAILWAY_PRIVATE_DOMAIN` and `LISTEN_PORT`
   variables. For example:

   ```sh
   MY_PRIVATE_TAILSCALE_SERVICE="http://{{railtail.RAILWAY_PRIVATE_DOMAIN}}:${{railtail.LISTEN_PORT}}"
   ```

Look at the [Examples](#examples) section for provider-specific examples.

## Configuration

railtail will forward TCP connections if you provide a `TARGET_ADDR` without
a `http://` or `https://` scheme. If you want railtail to act as an HTTP
proxy, ensure you have a `http://` or `https://` in your `TARGET_ADDR`.

| Environment Variable | CLI Argument       | Description                                                                                                                                                   |
| -------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TARGET_ADDR`        | `-target-addr`     | Required. Address of the Tailscale node to send traffic to.                                                                                                   |
| `LISTEN_PORT`        | `-listen-port`     | Required. Port to listen on.                                                                                                                                  |
| `TS_HOSTNAME`        | `-ts-hostname`     | Required. Hostname to use for Tailscale.                                                                                                                      |
| `TS_AUTH_KEY`        | N/A                | Required. Tailscale auth key. Must be set in environment.                                                                                                     |
| `TS_LOGIN_SERVER`    | `-ts-login-server` | Optional. Base URL of the control server. If you are using Headscale for your control server, use your Headscale instance's url. Defaults to using Tailscale. |
| `TS_STATEDIR_PATH`   | `-ts-state-dir`    | Optional. Tailscale state dir. Defaults to `/tmp/railtail`.                                                                                                   |
| `INSECURE_SKIP_VERIFY` | `-insecure-skip-verify` | Optional. Skip TLS certificate verification of an `https://` target. Defaults to `false`. Only enable for a target with a self-signed certificate you cannot replace. |
| `KEEPALIVE_INTERVAL` | `-keepalive-interval` | Optional. How often to send a little traffic to the target so the Tailscale path stays up between requests. Defaults to `60s`; `0` disables it. See [Keep-alive](#keep-alive). |
| `KEEPALIVE_PATH` | `-keepalive-path` | Optional. HTTP mode only: the path requested on the target's host by the keep-alive. Defaults to `/`. Choose something cheap to answer; any status code counts. |

_CLI arguments will take precedence over environment variables._

### Security notes

- railtail does not authenticate its callers: anything that can reach
  `LISTEN_PORT` can reach `TARGET_ADDR`. Keep it on Railway's Private Network
  and give the Tailscale node a tag with ACLs that allow it to reach only the
  target.
- Prefer a tagged, single-use or ephemeral auth key. With the default
  `TS_STATEDIR_PATH` in `/tmp`, the node identity is lost on every redeploy
  and the service re-registers with `TS_AUTH_KEY` - so a single-use key only
  works if `TS_STATEDIR_PATH` is on a volume. Otherwise the second deploy
  waits at `NeedsLogin` forever and every request through it hangs.
- The image runs as the distroless `nonroot` user. If you mount a Railway
  volume for `TS_STATEDIR_PATH`, it must be writable by that user (for
  example set `RAILWAY_RUN_UID=0`).

### Keep-alive

A Tailscale path that carries nothing for a couple of minutes has to be
re-established before the next packet gets through. Against a node reached
through a DERP relay, that made the first request after two and a half
minutes of quiet take ~620ms instead of ~200ms, and a caller whose traffic
comes in bursts paid it on almost every burst.

So railtail sends a little traffic on a timer (`KEEPALIVE_INTERVAL`, 60s by
default - under the ~2 minutes it takes the path to go cold):

- **HTTP mode** requests `KEEPALIVE_PATH` on the target's host through the
  same connection pool as forwarded traffic, which keeps both the Tailscale
  path and a pooled connection open. Any response counts, so a path that
  returns a quick 404 is ideal; avoid one that does real work.
- **TCP mode** opens and closes a connection to `TARGET_ADDR`.

A failing probe is logged once when it starts failing and once when it
recovers. The HTTP pool also keeps up to 16 idle connections to the target
(Go's default is 2, so concurrent requests beyond two paid a fresh handshake)
and closes them after 5 minutes unused.

## About

This was created to work around userspace networking restrictions. Dialing a
Tailscale node from a container requires you to do it over Tailscale's
local SOCKS5/HTTP proxy, which is not always ergonomical especially if
you're connecting to databases or other services with minimal support
for SOCKS5 (e.g. db connections from an application).

railtail is designed to be run as a separate service in Railway that you
connect to over Railway's Private Network.

> ⚠️ **Warning**: Do not expose this service on Railway publicly!
>
> ![Networking settings warning](https://res.cloudinary.com/railway/image/upload/v1733851092/cs-2024-12-11-01.12_f1z1xy.png)
>
> This service is intended to be used via Railway's Private Network only.

## Examples

### Connecting to an AWS RDS instance

1. Configure Tailscale on an EC2 instance in the same VPC as your RDS instance:

   ```sh
   # In EC2
   curl -fsSL https://tailscale.com/install.sh | sh

   # Enable IP forwarding
   echo 'net.ipv4.ip_forward = 1' | sudo tee -a /etc/sysctl.d/99-tailscale.conf
   echo 'net.ipv6.conf.all.forwarding = 1' | sudo tee -a /etc/sysctl.d/99-tailscale.conf
   sudo sysctl -p /etc/sysctl.d/99-tailscale.conf

   # Start Tailscale. Follow instructions to authenticate the node if needed,
   # and make sure you approve the subnet routes in the Tailscale admin console
   sudo tailscale up --reset --advertise-routes=172.31.0.0/16
   ```

2. Deploy railtail into your pre-existing Railway project:

   [![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/template/railtail?referralCode=EPXG5z)

3. Use your new railtail service's Private Domain to connect to your RDS instance:

   ```sh
   DATABASE_URL="postgresql://username:password@${{railtail.RAILWAY_PRIVATE_DOMAIN}}:${{railtail.LISTEN_PORT}}/dbname"
   ```
