# Diagrama entidad-relación

Sale de [`db/schema.sql`](../db/schema.sql); si cambia el esquema, hay que actualizar este archivo.
Primero va una vista general solo con las relaciones, y después el detalle de cada área con sus columnas.

Notación de las relaciones (pata de gallo): `||` exactamente uno, `|o` cero o uno, `o{` cero o muchos.
Una relación `|o` significa que la clave foránea admite `NULL`.

## Estado de cada tabla

| Área | Tabla | Estado |
|---|---|---|
| Usuarios | `users`, `sessions` | En uso |
| Usuarios | `follows` | Planificada |
| Clasificación | `genres`, `tags`, `story_genres`, `story_tags` | En uso |
| Historias | `stories`, `chapters` | En uso |
| Interacción | `comments` | En uso (solo comentarios en capítulos) |
| Interacción | `story_ratings` | Planificada |
| Biblioteca | `reading_progress` | En uso (registra las lecturas) |
| Biblioteca | `reading_lists`, `reading_list_items` | Planificada |
| Notificaciones | `notifications` | Planificada |
| Moderación | `reports`, `moderation_actions` | En uso |

## Vista general

```mermaid
erDiagram
    users ||--o{ sessions : "inicia"
    users ||--o{ follows : "sigue"
    users ||--o{ follows : "es seguido"
    users ||--o{ stories : "escribe"

    stories ||--o{ chapters : "contiene"
    stories ||--o{ story_genres : ""
    genres ||--o{ story_genres : ""
    stories ||--o{ story_tags : ""
    tags ||--o{ story_tags : ""

    users |o--o{ comments : "escribe"
    stories ||--o{ comments : "recibe"
    chapters |o--o{ comments : "recibe"
    comments |o--o{ comments : "responde a"
    users ||--o{ story_ratings : "valora"
    stories ||--o{ story_ratings : "recibe"

    users ||--o{ reading_lists : "tiene"
    reading_lists ||--o{ reading_list_items : "contiene"
    stories ||--o{ reading_list_items : "aparece en"
    users ||--o{ reading_progress : "lee"
    stories ||--o{ reading_progress : ""
    chapters |o--o{ reading_progress : "último leído"

    users ||--o{ notifications : "recibe"

    users |o--o{ reports : "reporta"
    stories |o--o{ reports : ""
    chapters |o--o{ reports : ""
    comments |o--o{ reports : ""
    users |o--o{ moderation_actions : "modera"
    users |o--o{ moderation_actions : "afectada"
    stories |o--o{ moderation_actions : ""
```

## Usuarios y sesiones

`username` y `email` son `citext` (sin distinguir mayúsculas). La suspensión está vigente si
`suspended_at` tiene valor y `suspended_until` es `NULL` (indefinida) o una fecha futura.

```mermaid
erDiagram
    users ||--o{ sessions : "inicia"
    users ||--o{ follows : "sigue (follower_id)"
    users ||--o{ follows : "es seguido (followed_id)"

    users {
        bigint id PK
        citext username UK "3 a 30: letras, números, _"
        citext email UK
        text password_hash "Argon2id"
        text display_name "nullable"
        text bio "máx. 1000"
        text avatar_url "nullable"
        text role "user | moderator | admin"
        text terms_version
        timestamptz terms_accepted_at
        timestamptz suspended_at "nullable"
        timestamptz suspended_until "NULL = indefinida"
        text suspension_reason
        timestamptz created_at
        timestamptz updated_at
    }
    sessions {
        text token_hash PK "SHA-256 del token"
        bigint user_id FK
        timestamptz expires_at "30 días"
        timestamptz created_at
    }
    follows {
        bigint follower_id PK, FK
        bigint followed_id PK, FK "distinto de follower_id"
        timestamptz created_at
    }
```

## Historias, capítulos y clasificación

Una historia tiene hasta 3 géneros y hasta 20 tags; `position` es el orden que eligió el autor.
`search` es un `tsvector` generado (título con más peso que la sinopsis).

```mermaid
erDiagram
    users ||--o{ stories : "escribe"
    stories ||--o{ chapters : "contiene"
    stories ||--o{ story_genres : ""
    genres ||--o{ story_genres : ""
    stories ||--o{ story_tags : ""
    tags ||--o{ story_tags : ""

    stories {
        bigint id PK
        bigint author_id FK "UK con slug"
        text title "1 a 200"
        text slug "UK con author_id"
        text synopsis "máx. 4000"
        text cover_url "nullable"
        text kind "original | fanfic"
        text language
        text content_rating "general | teen | mature"
        text_array warnings "text[]: none, declined, graphic_violence…"
        text status "draft | ongoing | completed | hiatus"
        timestamptz published_at
        timestamptz last_chapter_at
        bigint reads_count "desnormalizado"
        timestamptz hidden_at "oculta por moderación"
        text hidden_reason
        tsvector search "generado"
        timestamptz created_at
        timestamptz updated_at
    }
    chapters {
        bigint id PK
        bigint story_id FK "UK con position"
        integer position "UK diferible"
        text title "1 a 200"
        text content
        integer word_count
        boolean is_published
        timestamptz published_at
        timestamptz created_at
        timestamptz updated_at
    }
    genres {
        smallint id PK
        text name UK
        text slug UK
    }
    story_genres {
        bigint story_id PK, FK
        smallint genre_id PK, FK
        smallint position "1 a 3"
    }
    tags {
        bigint id PK
        text name "1 a 50"
        text slug UK "identidad del tag"
    }
    story_tags {
        bigint story_id PK, FK
        bigint tag_id PK, FK
        smallint position "1 a 20"
    }
```

## Interacción: comentarios y valoraciones

Un comentario con `chapter_id` `NULL` es un comentario a la historia (todavía no se usa). El
borrado es suave (`deleted_at`), y si se borra la cuenta, el comentario se queda sin autor.

```mermaid
erDiagram
    users |o--o{ comments : "escribe"
    stories ||--o{ comments : "recibe"
    chapters |o--o{ comments : "recibe"
    comments |o--o{ comments : "responde a (parent_id)"
    users ||--o{ story_ratings : "valora"
    stories ||--o{ story_ratings : "recibe"

    comments {
        bigint id PK
        bigint user_id FK "NULL si se borró la cuenta"
        bigint story_id FK
        bigint chapter_id FK "NULL = a la historia"
        bigint parent_id FK "NULL = comentario principal"
        text body "1 a 5000"
        timestamptz created_at
        timestamptz updated_at
        timestamptz deleted_at "borrado suave"
    }
    story_ratings {
        bigint user_id PK, FK
        bigint story_id PK, FK
        smallint score "1 a 5"
        timestamptz created_at
    }
```

## Biblioteca y progreso de lectura

Cada usuario tiene como máximo una lista `favorites` y una `library`; las `custom` no tienen límite.

```mermaid
erDiagram
    users ||--o{ reading_lists : "tiene"
    reading_lists ||--o{ reading_list_items : "contiene"
    stories ||--o{ reading_list_items : "aparece en"
    users ||--o{ reading_progress : "lee"
    stories ||--o{ reading_progress : ""
    chapters |o--o{ reading_progress : "último leído"

    reading_lists {
        bigint id PK
        bigint user_id FK "UK con name"
        text name "1 a 100"
        text kind "favorites | library | custom"
        boolean is_public
        timestamptz created_at
    }
    reading_list_items {
        bigint list_id PK, FK
        bigint story_id PK, FK
        timestamptz added_at
    }
    reading_progress {
        bigint user_id PK, FK
        bigint story_id PK, FK
        bigint chapter_id FK "nullable"
        timestamptz updated_at
    }
```

## Notificaciones

```mermaid
erDiagram
    users ||--o{ notifications : "recibe (user_id)"
    users |o--o{ notifications : "provoca (actor_id)"
    stories |o--o{ notifications : ""
    chapters |o--o{ notifications : ""
    comments |o--o{ notifications : ""

    notifications {
        bigint id PK
        bigint user_id FK "destinatario"
        text type "new_chapter, new_comment, comment_reply…"
        bigint actor_id FK "nullable"
        bigint story_id FK "nullable"
        bigint chapter_id FK "nullable"
        bigint comment_id FK "nullable"
        timestamptz read_at "NULL = sin leer"
        timestamptz created_at
    }
```

## Moderación

Un reporte apunta al menos a una historia, un capítulo o un comentario. El registro de acciones usa
`ON DELETE SET NULL` para sobrevivir aunque se borre la historia o la cuenta.

```mermaid
erDiagram
    users |o--o{ reports : "reporta (reporter_id)"
    users |o--o{ reports : "cierra (reviewed_by)"
    stories |o--o{ reports : ""
    chapters |o--o{ reports : ""
    comments |o--o{ reports : ""
    users |o--o{ moderation_actions : "modera (moderator_id)"
    users |o--o{ moderation_actions : "afectada (target_user_id)"
    stories |o--o{ moderation_actions : ""

    reports {
        bigint id PK
        bigint reporter_id FK
        bigint story_id FK
        bigint chapter_id FK
        bigint comment_id FK
        text reason "spam | plagiarism | harassment | inappropriate | other"
        text details "máx. 1000"
        text status "open | reviewing | resolved | dismissed"
        bigint reviewed_by FK
        timestamptz reviewed_at
        timestamptz created_at
    }
    moderation_actions {
        bigint id PK
        bigint moderator_id FK
        text action "hide_story, suspend_user, change_role…"
        bigint target_user_id FK
        bigint story_id FK
        text reason
        text details
        timestamptz created_at
    }
```
