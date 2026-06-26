# Production deploy — api.pacientesvenezuela.help

## Bootstrap inicial (una sola vez)

1. SSH como `deploy` y crear directorio:

   ```bash
   ssh deploy@<vps-host>
   mkdir -p /opt/apps/patient-finder
   cd /opt/apps/patient-finder
   ```

2. Crear `.env` desde el template:

   ```bash
   # local
   scp deploy/prod/.env.prod.example deploy@<vps-host>:/opt/apps/patient-finder/.env

   # remote
   ssh deploy@<vps-host>
   cd /opt/apps/patient-finder
   chmod 600 .env
   vim .env  # reemplazar todos los __REPLACE__
   ```

3. Login al registry:

   ```bash
   docker login registry.lupicrm.com
   # User: vps-pull
   ```

4. Verificar que la red `proxy` exista:

   ```bash
   docker network ls | grep proxy
   ```

5. Agregar Caddy block en el repo `infra` — ver `ansible/roles/caddy/templates/Caddyfile.j2`.

6. DNS en Cloudflare: A record `api.pacientesvenezuela.help` → IP del VPS.

## GitHub Secrets

| Secret | Valor |
|--------|-------|
| `REGISTRY_USERNAME` | push creds |
| `REGISTRY_PASSWORD` | push password |
| `STAGING_HOST` | VPS hostname |
| `DEPLOY_SSH_KEY` | private key del usuario `deploy` |

## Deploys subsecuentes

Automático: cada merge a `main` dispara el workflow.
