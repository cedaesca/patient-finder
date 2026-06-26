# Staging deploy — api.patient-finder.com

Esta carpeta contiene los artefactos necesarios para desplegar la API en el VPS
de staging detrás del Caddy gestionado por el repo `infra`.

## Modelo de deploy

- La imagen vive en `registry.patient-finder.com/patient-finder/patient-finder-api:<tag>`.
- En el VPS, los archivos viven en `/opt/apps/patient-finder-api/`. El código fuente no
   se materializa nunca en el VPS.
- Caddy corre como contenedor en una red Docker externa llamada `proxy` y
   resuelve `patient-finder-api:8080` por DNS interno. TLS y Let's Encrypt los maneja
  Caddy automáticamente.
- GitHub Actions construye, pushea, sincroniza el compose, y deploya en cada
  merge a `master` (ver `.github/workflows/deploy-staging.yml`). El operador
  no toca nada después del bootstrap inicial.

## Bootstrap inicial del VPS (una sola vez)

> El compose y el otel-collector-config NO se suben manualmente — CI los
> sincroniza en cada deploy via `scp`. Solo tienes que crear el directorio,
> el `.env`, y hacer login al registry.

1. SSH como `deploy` y crear el directorio de la app:
   ```bash
   ssh deploy@<vps-host>
    mkdir -p /opt/apps/patient-finder-api
    cd /opt/apps/patient-finder-api
   ```

2. Crear `.env` desde el template y rellenar los `__REPLACE__`:
   ```bash
   # Desde tu máquina:
    scp deploy/staging/.env.staging.example deploy@<vps-host>:/opt/apps/patient-finder-api/.env
    # En el VPS:
    ssh deploy@<vps-host>
    cd /opt/apps/patient-finder-api
   chmod 600 .env
   vim .env
   # Para generar secretos: openssl rand -base64 32
   ```

3. Login al registry desde el VPS — usar las credenciales **pull**
   (`vps-pull`, gestionadas en `infra/ansible/group_vars/all/vault.yml`,
   campos `vault_registry_pull_user` / `vault_registry_pull_password`):
   ```bash
    docker login registry.patient-finder.com
   # User: vps-pull
   # Password: <vault_registry_pull_password>
   ```

4. Verificar que la red `proxy` exista (la crea el repo `infra` durante el
   bootstrap):
   ```bash
   docker network ls | grep proxy
   ```

5. Agregar la entrada Caddy en el repo `infra`
   (`ansible/roles/caddy/templates/Caddyfile.j2`):
   ```caddy
    api.patient-finder.com {
        reverse_proxy patient-finder-api:8080
   }
   ```
   y aplicar:
   ```bash
   make caddy
   ```

## GitHub Secrets requeridos

Configurar en `Settings → Secrets and variables → Actions`:

| Secret | Valor |
|--------|-------|
| `REGISTRY_USERNAME` | `vault_registry_push_user` (credencial **push**) |
| `REGISTRY_PASSWORD` | `vault_registry_push_password` |
| `STAGING_HOST` | hostname o IP del VPS staging |
| `DEPLOY_SSH_KEY` | private key SSH del usuario `deploy` (formato OpenSSH completo) |

## Deploys subsecuentes

Automático: cada merge a `master` dispara el workflow, que:
1. Construye y pushea la imagen (`:<sha>` + `:latest`).
2. `mkdir -p /opt/apps/patient-finder-api` (idempotente).
3. `scp` del compose + otel config + alloy config al VPS.
4. SSH: `mv docker-compose.staging.yaml docker-compose.yml`, `docker compose pull && up -d --remove-orphans`, `docker image prune -f`.
5. El sidecar `migrate` corre automáticamente antes que la `api`.

Disparo manual: pestaña Actions → Deploy staging → Run workflow.

## Rollback

Dos opciones:

**A — Re-correr el workflow en un SHA anterior** (limpio):
GitHub Actions → Deploy staging → Run workflow → escoger commit/branch antiguo.

**B — Pinear manualmente en el host** (rápido):
```bash
ssh deploy@<vps-host>
cd /opt/apps/patient-finder-api
sed -i 's/^API_IMAGE_TAG=.*/API_IMAGE_TAG=<sha-anterior>/' .env
docker compose pull
docker compose up -d
```
Ojo: el próximo deploy de CI sobrescribe el compose pero NO el `.env`, así
que el pin sobrevive hasta que vuelvas a poner `API_IMAGE_TAG=latest`.

## Observability

Logs de todos los containers del stack se envían a **New Relic** vía Grafana Alloy
(scrape del Docker socket → OTLP gRPC → otel-collector → New Relic). La pipeline de
`logs/` ya existe en `otel-collector-config.yaml`; Alloy es el productor.

- Ver logs en `one.newrelic.com → Logs` con filtro `env = "staging"`.
- `service` (ej. `api`, `psql`, `centrifugo`) y `container` están como attributes
  para filtrar por componente.
- `trace_id` y `span_id` viajan como structured metadata — clic en un `trace_id`
  desde un log lleva al trace correspondiente.

Si los logs dejan de aparecer, troubleshoot del agente:
```bash
ssh deploy@<vps-host>
cd /opt/apps/patient-finder-api
docker compose logs alloy
```

## Inspección y debugging

- Logs en vivo: `docker logs -f patient-finder-api`
- Estado del stack: `cd /opt/apps/patient-finder-api && docker compose ps`
- Mailpit inbox (vía SSH tunnel desde tu máquina):
  ```bash
  ssh -L 8025:patient-finder-api-mailpit-1:8025 deploy@<vps-host>
  # luego en tu browser: http://localhost:8025
  ```
- Postgres CLI:
  ```bash
  cd /opt/apps/patient-finder-api
  docker compose exec psql psql -U "$DB_USERNAME" -d "$DB_DATABASE"
  ```

## Estructura de archivos

| Archivo | Propósito | ¿Se sincroniza por CI? |
|---------|-----------|------------------------|
| `docker-compose.staging.yaml` | Stack completo (api + migrate + psql + otel + alloy + centrifugo + mailpit) | Sí — CI lo `scp`'ea cada deploy |
| `otel-collector-config.yaml` | Config del OTEL collector que reenvía a New Relic | Sí — CI lo `scp`'ea cada deploy |
| `../observability/alloy/config.staging.alloy` | Config de Alloy (scrape Docker → OTLP) | Sí — CI lo `scp`'ea a `${APP_DIR}/alloy/` |
| `.env.staging.example` | Plantilla del `.env` con todas las keys requeridas | No — solo referencia local |
