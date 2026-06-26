# Dashboard de Data Entry — User Stories

> Sistema de búsqueda de pacientes/evacuados post-terremoto La Guaira 7.7.
> Voluntarios de confianza digitalizan listas manuscritas.
> Público general busca familiares.

---

## 1. Autenticación simplificada

**Como** voluntario,
**Quiero** iniciar sesión con mi email y contraseña,
**Para** acceder al dashboard.

- El administrador crea mi cuenta con un email y contraseña temporal
- Yo hago login con email + contraseña
- Puedo cambiar mi contraseña desde el perfil

### Pantallas
- **Login**: formulario email + contraseña, botón "Iniciar sesión"
- Si credenciales incorrectas → mensaje de error
- Si ok → redirige al dashboard

---

## 2. Roles y permisos

El sistema tiene 5 roles. Cada usuario puede tener uno o varios roles. Los roles globales aplican a todos los centros. Los roles por centro aplican solo al centro asignado.

### Roles globales

| Rol | Acceso |
|-----|--------|
| **Administrador** | Todo: gestionar pacientes, centros, usuarios y auditoría |
| **Supervisor** | Todo excepto gestionar usuarios (no crea, edita ni elimina voluntarios) |

### Roles por centro

| Rol | Qué puede hacer en su centro |
|-----|------------------------------|
| **Encargado de Centro** | Crear, editar y eliminar pacientes |
| **Digitador** | Crear y editar pacientes (no eliminar) |
| **Registrador** | Solo crear pacientes |

### Cómo afecta al dashboard
- La barra lateral y los botones cambian según el rol del usuario
- **Admin/Supervisor**: ven las secciones Pacientes, Centros, Usuarios y Auditoría
- **Encargado/Digitador/Registrador**: ven solo Pacientes (y su centro está fijo)
- Si un usuario tiene rol en múltiples centros, puede alternar entre ellos

---

## 3. Dashboard — Lista de pacientes

**Como** usuario autenticado,
**Quiero** ver los pacientes en una tabla paginada,
**Para** revisar registros existentes y gestionar la base de datos.

### Pantalla

| Elemento | Descripción |
|----------|-------------|
| Tabla | Columnas: nombre, cédula, sexo, edad, estado, centro, fecha ingreso |
| Búsqueda | Input que busca por nombre o cédula |
| Filtros | Sexo, estado de rescate, centro |
| Paginación | 20 por página |
| Botón "Nuevo paciente" | Siempre visible si el rol puede crear |
| Botón "Editar" | Por fila, visible si el rol puede editar |
| Botón "Eliminar" | Por fila, visible si el rol puede eliminar |

### Comportamiento por rol
- **Admin / Supervisor**: ven pacientes de TODOS los centros
- **Encargado / Digitador / Registrador**: ven solo pacientes de su(s) centro(s) asignado(s)

---

## 4. Crear paciente

**Como** usuario autenticado con permiso de crear,
**Quiero** llenar un formulario para registrar un paciente,
**Para** digitalizar las listas manuscritas.

### Pantalla: Formulario de paciente

| Campo | Tipo | Requerido |
|-------|------|-----------|
| Primer nombre | texto | si* |
| Apellido | texto | si* |
| Cédula | texto | si* |
| Sexo | select | no |
| Edad aproximada | número | no |
| Estado (status) | select | sí |
| Fecha de ingreso | fecha | sí |
| Estado de rescate | select en cascada | sí |
| Municipio | select en cascada | sí |
| Parroquia | select en cascada | no |
| Centro | select | sí |
| Contactos | texto | no |
| Notas | área de texto | no |

> *Debe tener cédula O (nombre + apellido)

### Comportamiento por rol
- **Admin / Supervisor**: pueden seleccionar cualquier centro
- **Roles por centro**: el centro se preselecciona automáticamente y no se puede cambiar

### Selects en cascada
- Al seleccionar un estado, se cargan sus municipios
- Al seleccionar un municipio, se cargan sus parroquias

---

## 5. Editar paciente

**Como** usuario con permiso de editar,
**Quiero** modificar los datos de un paciente existente,
**Para** corregir errores o actualizar su estado.

### Pantalla: mismo formulario que creación, precargado
- Todos los campos editables
- Botón "Guardar cambios"
- Botón "Cancelar"

---

## 6. Eliminar paciente

**Como** usuario con permiso de eliminar,
**Quiero** eliminar un paciente duplicado o incorrecto,
**Para** mantener la base de datos limpia.

- Botón "Eliminar" con confirmación: "¿Estás seguro de eliminar a [nombre]?"
- Soft delete: desaparece del dashboard y búsqueda pública

---

## 7. Gestionar centros

**Como** administrador o supervisor,
**Quiero** crear, editar y desactivar centros,
**Para** mantener la lista actualizada.

### Pantalla: Lista de centros
- Tabla con nombre, tipo, ubicación, activo/inactivo
- Botones: Nuevo centro, Editar, Desactivar

### Pantalla: Formulario de centro

| Campo | Requerido |
|-------|-----------|
| Nombre | sí |
| Tipo (Hospital / Albergue / Morgue) | sí |
| Estado | sí |
| Municipio | sí |
| Parroquia | no |
| Dirección | no |
| Contactos | no |

---

## 8. Gestionar usuarios

**Como** administrador,
**Quiero** crear, editar y desactivar voluntarios,
**Para** controlar quién accede al sistema.

### Pantalla: Lista de voluntarios
- Tabla con: nombre, email, roles, activo/inactivo
- Botones: Nuevo voluntario, Editar, Desactivar

### Pantalla: Crear / Editar voluntario
- Email, nombre, contraseña temporal
- Asignación de roles (globales o por centro)

---

## 9. Perfil

**Como** cualquier usuario autenticado,
**Quiero** ver mi perfil y cambiar mi contraseña.

- Ver mi nombre y email
- Formulario: contraseña actual + nueva + confirmar

---

## Resumen de pantallas

| # | Pantalla | Quién ve |
|---|----------|----------|
| 1 | Login | Todos |
| 2 | Dashboard (pacientes) | Todos |
| 3 | Crear paciente | Todos los roles |
| 4 | Editar paciente | Admin, Supervisor, Encargado, Digitador |
| 5 | Eliminar paciente | Admin, Supervisor, Encargado |
| 6 | Gestionar centros | Admin, Supervisor |
| 7 | Gestionar usuarios | Solo Admin |
| 8 | Auditoría | Admin, Supervisor |
| 9 | Perfil | Todos |
