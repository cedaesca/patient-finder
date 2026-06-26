# Observability — Logs

Logs de todos los containers del compose se envían a **New Relic** vía Grafana Alloy
y el OTel collector existente. Traces + metrics ya viajan por el OTel collector
(no es nuevo); logs son el pipeline que se sumó acá.

## Pipeline

```
                                     ┌──────────────────────┐
   Container stdout (JSON slog) ───► │  Alloy               │
                                     │  - discovery.docker  │
                                     │  - parse JSON        │
                                     │  - bridge a OTel     │
                                     └──────────┬───────────┘
                                                │ OTLP gRPC :4317
                                                ▼
                                     ┌──────────────────────┐
                                     │  otel-collector      │ ◄── traces + metrics
                                     │  (pipeline logs/)    │     (de la app Go)
                                     └──────────┬───────────┘
                                                │ OTLP HTTP
                                                ▼
                                          New Relic
                                  one.newrelic.com → Logs
```

Alloy se conecta al Docker socket (no parsea filesystem), descubre containers con
label `com.docker.compose.project=patient-finder-api`, parsea el JSON de slog y mapea
`level` → severity OTel y `trace_id`/`span_id`/`request_id` → attributes.

## Configuración por environment

| Env | Archivo | Retention NR | Acceso |
|-----|---------|--------------|--------|
| dev | `alloy/config.dev.alloy` | NR Free tier | UI Alloy local en `http://localhost:12345` |
| staging | `alloy/config.staging.alloy` | NR Free tier | UI Alloy solo via SSH tunnel |

La diferencia entre los dos archivos es una sola línea: el label `env`. Decidimos
no abstraer (un base + override) porque la duplicación es trivial y mantener dos
archivos completos es más legible.

## Operaciones comunes

### Buscar logs en NR

`one.newrelic.com → Logs`. Filtros típicos:
- `env = "dev"` o `env = "staging"`
- `service = "api"` (o `psql`, `centrifugo`, `mailpit`, etc.)
- `level = "error"`
- Click en `trace_id` desde un log para saltar al trace correspondiente.

### Añadir un servicio nuevo al compose

Nada que hacer en Alloy. El filtro `com.docker.compose.project=patient-finder-api` captura
cualquier container del stack automáticamente. Si tu nuevo servicio loguea JSON con
campos `level`/`msg`/`trace_id`/`request_id`, los va a parsear igual; si loguea
texto plano, se envía sin parsear (`stage.json` falla silenciosamente y el log
sigue de largo).

### Deshabilitar Alloy temporalmente

Si necesitás reducir ruido en NR durante un soak / experimento:

```bash
# dev
make alloy-down

# staging
ssh deploy@<vps-host>
cd /opt/apps/patient-finder-api && docker compose stop alloy
```

Para reactivarlo: `make alloy-up` (dev) o `docker compose start alloy` (staging).

### Filtrar logs ruidosos

Si NR ingest se acerca al free tier (100 GB/mes), se puede dropear logs específicos
en el `loki.process` antes del bridge OTel. Patrón:

```hcl
loki.process "app" {
  forward_to = [otelcol.receiver.loki.bridge.receiver]

  // Drop healthchecks
  stage.match {
    selector = "{service=\"api\"}"
    stage.regex {
      expression = ".*GET /health.*"
    }
    stage.drop {}
  }

  // ... resto del pipeline
}
```

Aplicar primero en `config.dev.alloy`, validar que el log no aparece más en NR (esperar ~1 min),
y replicar en `config.staging.alloy`.

### Troubleshoot

```bash
# dev
make alloy-logs

# staging
ssh deploy@<vps-host>
cd /opt/apps/patient-finder-api && docker compose logs -f alloy
```

Errores típicos:
- `DeadlineExceeded` → otel-collector no responde a tiempo. El exporter ya tiene
  `sending_queue` y `retry_on_failure`, así que es transitorio (típico al boot
  cuando todos los containers loguean en burst). Si persiste varios minutos,
  revisar `docker compose logs otel-collector`.
- Si el container de un service no aparece en NR: verificar que tiene la label
  `com.docker.compose.project=patient-finder-api` con `docker inspect <container> --format '{{json .Config.Labels}}'`.
