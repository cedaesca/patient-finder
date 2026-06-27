# Patient & Evacuee Locator API

Backend para agregar listas de pacientes/refugios/morgues después del terremoto 7.7 de La Guaira, Venezuela.

## Resumen de la implementación

El sistema permite a voluntarios digitalizar listas manuscritas de pacientes en centros médicos y provee una API de búsqueda pública para que familias encuentren a sus seres queridos.

**Stack:** Go (chi router), PostgreSQL, Typesense (búsqueda), JWT auth, OpenTelemetry.

**Arquitectura:** Handlers → Services → Stores → PostgreSQL con inyección de dependencias.

### Módulos

| Módulo      | Propósito                                                      |
| ----------- | -------------------------------------------------------------- |
| `auth`      | Login con email+password, JWT access+refresh tokens            |
| `users`     | CRUD de usuarios, gestión de roles, cambio de password con OTP |
| `roles`     | Catálogo de roles y permisos, verificación de autorización     |
| `audit`     | Log inmutable de todas las operaciones de escritura            |
| `geography` | Estados, municipios y parroquias de Venezuela (solo lectura)   |
| `centers`   | CRUD de centros médicos con validación geográfica              |
| `persons`   | CRUD de pacientes con búsqueda fuzzy, permisos por centro      |

### Modelo de permisos

Los permisos están asociados a roles. Hay dos tipos de roles:

- **Roles globales** (admin, supervisor): el usuario tiene el permiso en **todos** los centros.
- **Roles por centro** (encargado, digitador, registrador): el usuario solo tiene el permiso en **ese centro específico**.

| Rol           | Global | Permisos                                                   |
| ------------- | ------ | ---------------------------------------------------------- |
| `admin`       | Sí     | Todos (`patients:*`, `centers:*`, `users:*`, `audit:read`) |
| `supervisor`  | Sí     | `patients:*`, `centers:*` (sin `users:*`)                  |
| `encargado`   | No     | `patients:*` (solo en su centro)                           |
| `digitador`   | No     | `patients:read`, `patients:create`, `patients:update`      |
| `registrador` | No     | `patients:read`, `patients:create`                         |

### Autenticación

Todas las rutas privadas requieren un token JWT en el header `Authorization: Bearer <token>`. El token se obtiene mediante login y se refresca con el endpoint `/auth/refresh`.

---

## Endpoints

### Auth

#### `POST /auth/login`

Inicia sesión y obtiene tokens JWT.

**Request:**

```json
{
  "email": "admin@ejemplo.com",
  "password": "secreto123"
}
```

**Response `200`:**

```json
{
  "data": {
    "access_token": "eyJhbGci...",
    "refresh_token": "eyJhbGci...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Response `401`:**

```json
{ "message": "invalid credentials" }
```

#### `POST /auth/refresh`

Refresca el access token usando el refresh token.

**Request:**

```json
{
  "refresh_token": "eyJhbGci..."
}
```

**Response `200`:** (mismo formato que login)

---

### Users (privado — requiere autenticación)

#### `GET /users/me`

Obtiene el perfil del usuario autenticado.

**Response `200`:**

```json
{
  "data": {
    "user": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Juan",
      "last_name": "Pérez",
      "email": "juan@ejemplo.com",
      "created_at": "2025-06-26T10:00:00Z",
      "updated_at": "2025-06-26T10:00:00Z",
      "last_activity_at": "2025-06-26T12:00:00Z",
      "deleted_at": null
    }
  }
}
```

**Response `200`:** `{}`

#### `GET /users/me/roles`

Obtiene los roles del usuario autenticado.

**Response `200`:**

```json
{
  "data": {
    "roles": [
      {
        "user_id": "550e8400-...",
        "role_id": "660e8400-...",
        "role_name": "admin",
        "is_global": true,
        "center_id": null,
        "created_at": "2025-06-26T10:00:00Z"
      }
    ]
  }
}
```

#### `POST /users`

Crea un nuevo usuario (requiere permiso `users:create`).

**Request:**

```json
{
  "email": "nuevo@ejemplo.com",
  "name": "María",
  "last_name": "González",
  "password": "contraseña123"
}
```

**Response `201`:**

```json
{
  "data": {
    "user": {
      "id": "770e8400-...",
      "name": "María",
      "last_name": "González",
      "email": "nuevo@ejemplo.com",
      "created_at": "2025-06-26T10:00:00Z",
      "updated_at": "2025-06-26T10:00:00Z",
      "last_activity_at": "2025-06-26T10:00:00Z",
      "deleted_at": null
    }
  }
}
```

#### `GET /users`

Lista todos los usuarios (requiere permiso `users:read`).

**Query params:** `page` (default 1), `page_size` (default 20, max 100)

**Response `200`:**

```json
{
  "data": {
    "users": [
      { "id": "...", "name": "Juan", "last_name": "Pérez", "email": "juan@ejemplo.com", ... }
    ]
  },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_records": 1,
    "total_pages": 1
  }
}
```

#### `GET /users/{id}`

Obtiene un usuario por ID.

**Response `200`:** (mismo formato que GET /users/me)

#### `PUT /users/{id}`

Actualiza un usuario (requiere permiso `users:update`). No se pueden modificar usuarios admin a menos que el actor sea el mismo usuario.

**Request:**

```json
{
  "name": "Juan Carlos",
  "last_name": "Pérez López",
  "email": "nuevoemail@ejemplo.com"
}
```

**Response `200`:** (usuario actualizado)

#### `DELETE /users/{id}`

Elimina un usuario (soft delete, requiere permiso `users:delete`). No se pueden eliminar usuarios admin.

**Response `204`:** (sin cuerpo)

#### `GET /users/{id}/roles`

Obtiene los roles de un usuario.

**Response `200`:** (mismo formato que GET /users/me/roles)

#### `PUT /users/{id}/roles`

Reemplaza todos los roles de un usuario (requiere permiso `users:update`).

**Request:**

```json
{
  "roles": [
    { "role_name": "digitador", "center_id": "880e8400-..." },
    { "role_name": "supervisor" }
  ]
}
```

**Response `200`:** (roles actualizados)

---

### Roles

#### `GET /roles`

Lista todos los roles disponibles.

**Response `200`:**

```json
{
  "data": {
    "roles": [
      {
        "id": "660e8400-...",
        "name": "admin",
        "display_name": "Administrador",
        "is_global": true,
        "permissions": [
          "patients:read",
          "patients:create",
          "patients:update",
          "patients:delete",
          "centers:read",
          "centers:create",
          "centers:update",
          "centers:delete",
          "users:read",
          "users:create",
          "users:update",
          "users:delete",
          "audit:read"
        ]
      },
      {
        "id": "770e8400-...",
        "name": "digitador",
        "display_name": "Digitador",
        "is_global": false,
        "permissions": ["patients:read", "patients:create", "patients:update"]
      }
    ]
  }
}
```

---

### Geography (público)

#### `GET /states`

Lista todos los estados de Venezuela.

**Response `200`:**

```json
{
  "data": {
    "states": [
      { "id": "a1b2c3d4-...", "name": "La Guaira", "created_at": "..." },
      { "id": "e5f6g7h8-...", "name": "Distrito Capital", "created_at": "..." }
    ]
  }
}
```

#### `GET /states/{id}`

Obtiene un estado por ID.

#### `GET /states/{id}/municipalities`

Lista municipios de un estado.

**Response `200`:**

```json
{
  "data": {
    "municipalities": [
      {
        "id": "m1m2m3m4-...",
        "name": "Vargas",
        "estado_id": "a1b2c3d4-...",
        "created_at": "..."
      }
    ]
  }
}
```

#### `GET /municipalities/{id}`

Obtiene un municipio por ID.

#### `GET /municipalities/{id}/parishes`

Lista parroquias de un municipio.

**Response `200`:**

```json
{
  "data": {
    "parishes": [
      {
        "id": "p1p2p3p4-...",
        "name": "La Guaira",
        "municipio_id": "m1m2m3m4-...",
        "created_at": "..."
      }
    ]
  }
}
```

#### `GET /parishes/{id}`

Obtiene una parroquia por ID.

---

### Centers (lectura pública, escritura privada)

#### `GET /centers`

Lista centros activos.

**Query params:** `page` (default 1), `page_size` (default 20, max 100)

**Response `200`:**

```json
{
  "data": {
    "centers": [
      {
        "id": "c1c2c3c4-...",
        "name": "Hospital José María Vargas",
        "type": "health",
        "estado_id": "a1b2c3d4-...",
        "municipio_id": "m1m2m3m4-...",
        "parroquia_id": "p1p2p3p4-...",
        "address": "Av. Principal, La Guaira",
        "contacts": "+58 212-1234567",
        "is_active": true,
        "created_at": "2025-06-26T10:00:00Z",
        "updated_at": "2025-06-26T10:00:00Z"
      }
    ]
  },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_records": 1,
    "total_pages": 1
  }
}
```

#### `GET /centers/{id}`

Obtiene un centro por ID.

**Response `200`:** (mismo formato que un item de la lista)

#### `POST /centers` (privado — requiere `centers:create`)

Crea un centro nuevo. Valida la jerarquía geográfica (estado → municipio → parroquia).

**Request:**

```json
{
  "name": "Hospital General del Oeste",
  "type": "health",
  "estado_id": "a1b2c3d4-...",
  "municipio_id": "m1m2m3m4-...",
  "parroquia_id": "p1p2p3p4-...",
  "address": "Av. Principal",
  "contacts": "+58 212-1234567"
}
```

**Response `201`:** (centro creado)

**Response `422`:**

```json
{
  "message": "validate municipio belongs to estado: invalid foreign key reference"
}
```

#### `PUT /centers/{id}` (privado — requiere `centers:update`)

Actualiza un centro (PATCH-style, solo campos enviados).

**Request:**

```json
{
  "name": "Nuevo nombre del hospital",
  "contacts": "+58 212-9999999"
}
```

**Response `200`:** (centro actualizado)

#### `DELETE /centers/{id}` (privado — requiere `centers:delete`)

Desactiva un centro (soft delete).

**Response `204`:** (sin cuerpo)

---

### Persons (lectura pública, escritura privada)

#### `GET /persons/search`

Búsqueda fuzzy de pacientes via Typesense.

**Query params:**

- `q` — término de búsqueda (nombre, cédula, etc.)
- `sex` — filtro por sexo (`M` o `F`)
- `estado_id` — filtro por estado de rescate
- `municipio_id` — filtro por municipio de rescate
- `parroquia_id` — filtro por parroquia de rescate
- `page` (default 1), `page_size` (default 20, max 100)

**Response `200`:**

```json
{
  "data": {
    "persons": [
      {
        "id": "d1d2d3d4-...",
        "first_name": "Carlos",
        "last_name": "Rodríguez",
        "cedula": "12345678",
        "sex": "M",
        "age_approx": 45,
        "status": "hospitalized",
        "admitted_at": "2025-06-26T10:00:00Z",
        "rescue_estado": "La Guaira",
        "rescue_municipio": "Vargas",
        "rescue_parroquia": "La Guaira",
        "center": {
          "id": "c1c2c3c4-...",
          "name": "Hospital José María Vargas",
          "type": "health",
          "contacts": "+58 212-1234567"
        },
        "notes": "Observaciones del paciente",
        "contacts": "+58 412-1234567",
        "created_at": "2025-06-26T10:00:00Z"
      }
    ]
  },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_records": 1,
    "total_pages": 1
  }
}
```

#### `GET /persons` (privado — requiere `patients:read`)

Lista paginada de pacientes con filtros opcionales. Solo para usuarios autenticados.

**Query params:**
- `page` (default 1), `page_size` (default 20, max 100)
- `center_id` — filtra por centro (UUID)
- `estado_id` — filtra por estado de rescate (UUID)
- `municipio_id` — filtra por municipio de rescate (UUID)
- `parroquia_id` — filtra por parroquia de rescate (UUID)

**Permisos:**
- Admin/Supervisor: pueden listar sin `center_id` (ven todos) o con `center_id` (ven ese centro)
- Encargado/Digitador/Registrador: deben especificar `center_id` de su centro asignado

**Response `200`:**

```json
{
  "data": {
    "persons": [
      {
        "id": "d1d2d3d4-...",
        "first_name": "Carlos",
        "last_name": "Rodríguez",
        "cedula": "12345678",
        "sex": "M",
        "age_approx": 45,
        "status": "hospitalized",
        "admitted_at": "2025-06-26T10:00:00Z",
        "rescue_estado": "La Guaira",
        "rescue_municipio": "Vargas",
        "rescue_parroquia": "La Guaira",
        "center": {
          "id": "c1c2c3c4-...",
          "name": "Hospital José María Vargas",
          "type": "health",
          "contacts": "+58 212-1234567"
        },
        "notes": "Observaciones del paciente",
        "contacts": "+58 412-1234567",
        "created_at": "2025-06-26T10:00:00Z"
      }
    ]
  },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_records": 150,
    "total_pages": 8
  }
}
```

**Response `403`:**
```json
{ "message": "Forbidden" }
```

#### `GET /persons/{id}`

Obtiene un paciente por ID.

**Response `200`:** (mismo formato que un item de búsqueda)

#### `POST /persons` (privado — requiere `patients:create` en el centro)

Crea un nuevo paciente. Valida la jerarquía geográfica de rescate.

**Request:**

```json
{
  "first_name": "Carlos",
  "last_name": "Rodríguez",
  "cedula": "12345678",
  "sex": "M",
  "age_approx": 45,
  "status": "hospitalized",
  "admitted_at": "2025-06-26T10:00:00Z",
  "rescue_estado_id": "a1b2c3d4-...",
  "rescue_municipio_id": "m1m2m3m4-...",
  "rescue_parroquia_id": "p1p2p3p4-...",
  "center_id": "c1c2c3c4-...",
  "contacts": "+58 412-1234567",
  "notes": "Paciente ingresado por emergencia"
}
```

**Campos requeridos:** `rescue_estado_id`, `rescue_municipio_id`, `center_id`

**Campos opcionales:** `first_name`, `last_name`, `cedula`, `sex`, `age_approx`, `status`, `admitted_at`, `rescue_parroquia_id`, `contacts`, `notes`, `source`, `source_id`

**Valores válidos:**

- `sex`: `"M"`, `"F"`
- `status`: `"hospitalized"`, `"discharged"`, `"deceased"`, `"transferred"` (default: `"hospitalized"`)
- `admitted_at`: formato RFC3339 (`"2025-06-26T10:00:00Z"`)

**Response `201`:**

```json
{
  "data": {
    "person": {
      "id": "d1d2d3d4-...",
      "first_name": "Carlos",
      "last_name": "Rodríguez",
      "cedula": "12345678",
      "sex": "M",
      "age_approx": 45,
      "status": "hospitalized",
      "admitted_at": "2025-06-26T10:00:00Z",
      "rescue_estado": "La Guaira",
      "rescue_municipio": "Vargas",
      "rescue_parroquia": "La Guaira",
      "center": {
        "id": "c1c2c3c4-...",
        "name": "Hospital José María Vargas",
        "type": "health",
        "contacts": "+58 212-1234567"
      },
      "notes": "Paciente ingresado por emergencia",
      "contacts": "+58 412-1234567",
      "created_at": "2025-06-26T10:00:00Z"
    }
  }
}
```

#### `PATCH /persons/{id}` (privado — requiere `patients:update` en el centro del paciente)

Actualiza un paciente (PATCH-style, solo campos enviados). Si se cambia el `center_id`, también se verifica permiso en el nuevo centro.

**Request:**

```json
{
  "status": "discharged",
  "notes": "Paciente dado de alta"
}
```

**Response `200`:** (paciente actualizado)

#### `DELETE /persons/{id}` (privado — requiere `patients:delete` en el centro del paciente)

Elimina un paciente (soft delete).

**Response `204`:** (sin cuerpo)

---

### Audit (privado — requiere autenticación)

#### `GET /audit`

Lista eventos de auditoría.

**Query params:**

- `page`, `page_size` — paginación
- `user_id` — filtrar por usuario
- `action` — filtrar por acción (`create`, `update`, `delete`)
- `resource_type` — filtrar por tipo (`user`, `center`, `person`, etc.)
- `resource_id` — filtrar por recurso específico
- `search` — búsqueda en datos
- `from`, `to` — rango de fechas (RFC3339)

**Response `200`:**

```json
{
  "data": {
    "events": [
      {
        "id": "a1a2a3a4-...",
        "user_id": "550e8400-...",
        "action": "create",
        "resource_type": "person",
        "resource_id": "d1d2d3d4-...",
        "before_data": null,
        "after_data": {
          "first_name": "Carlos",
          "last_name": "Rodríguez",
          "status": "hospitalized",
          "center_id": "c1c2c3c4-..."
        },
        "summary": "Creó persona",
        "created_at": "2025-06-26T10:00:00Z"
      }
    ]
  },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_records": 1,
    "total_pages": 1
  }
}
```

#### `GET /audit/resource-types`

Lista tipos de recursos con conteo de eventos.

**Response `200`:**

```json
{
  "data": {
    "resource_types": [
      { "resource_type": "person", "label": "persona", "count": 150 },
      { "resource_type": "user", "label": "usuario", "count": 5 }
    ]
  }
}
```

---

## Respuestas de error comunes

| Código | Significado                                              | Ejemplo                                                                                     |
| ------ | -------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `400`  | Bad Request — payload inválido o parámetros mal formados | `{ "message": "invalid request body" }`                                                     |
| `401`  | Unauthorized — token ausente o inválido                  | `{ "message": "Unauthorized" }`                                                             |
| `403`  | Forbidden — sin permiso para la operación                | `{ "message": "Forbidden" }`                                                                |
| `404`  | Not Found — recurso no existe                            | `{ "message": "person not found" }`                                                         |
| `409`  | Conflict — email/nombre duplicado                        | `{ "message": "the email is already taken" }`                                               |
| `422`  | Unprocessable Entity — referencia geográfica inválida    | `{ "message": "validate municipio belongs to estado: invalid rescue geography reference" }` |
| `500`  | Internal Server Error                                    | `{ "message": "internal server error" }`                                                    |

---

## Paginación

Todas las respuestas de lista incluyen un objeto `meta`:

```json
{
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_records": 150,
    "total_pages": 8
  }
}
```

---

## Headers

| Header          | Requerido      | Descripción             |
| --------------- | -------------- | ----------------------- |
| `Authorization` | Rutas privadas | `Bearer <access_token>` |
| `Content-Type`  | POST/PUT       | `application/json`      |
| `Accept`        | Opcional       | `application/json`      |

---

## CORS

Por defecto permite todos los orígenes (`https://*`, `http://*`). Se puede restringir con la variable de entorno `CORS_ALLOWED_ORIGINS` (lista separada por comas).

---

## Rate Limiting

- **Login:** 5 requests / 5 min, burst 3/min, con retry-after
- **Refresh:** mismo límite que login
- **OTP password:** 1 request/min, 4/hora por usuario e IP
- **Cambio de password:** 3 requests/min, 6/hora por usuario e IP

Los headers de rate limit se exponen en la respuesta (`X-RateLimit-*`).
