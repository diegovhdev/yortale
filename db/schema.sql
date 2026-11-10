-- =====================================================================
-- Plataforma de lectura y publicación de historias — Esquema PostgreSQL
-- Requiere PostgreSQL 13+
-- =====================================================================

CREATE EXTENSION IF NOT EXISTS citext;   -- texto sin distinción de mayúsculas (emails, usernames)
CREATE EXTENSION IF NOT EXISTS pg_trgm;  -- búsqueda aproximada por título ("harri poter" → "Harry Potter")

-- Función genérica para mantener updated_at
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- ---------------------------------------------------------------------
-- USUARIOS Y PERFILES
-- ---------------------------------------------------------------------
CREATE TABLE users (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username           citext NOT NULL UNIQUE
                       CHECK (username ~ '^[A-Za-z0-9_]{3,30}$'),
    email              citext NOT NULL UNIQUE,
    password_hash      text   NOT NULL,              -- argon2 / bcrypt, nunca texto plano
    display_name       text,
    bio                text   CHECK (char_length(bio) <= 1000),
    avatar_url         text,
    role               text   NOT NULL DEFAULT 'user'
                       CHECK (role IN ('user', 'moderator', 'admin')),
    terms_version      text,                         -- versión de términos aceptada
    terms_accepted_at  timestamptz,
    -- Suspensión por moderación. Está vigente si suspended_at no es NULL y suspended_until es
    -- NULL (indefinida) o futura. No se usa 'infinity' porque pgx no lo lee en un time.Time.
    suspended_at       timestamptz,
    suspended_until    timestamptz,
    suspension_reason  text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (suspended_until IS NULL OR suspended_at IS NOT NULL)
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Sesiones del lado del servidor (encaja bien con una app renderizada en servidor + htmx)
CREATE TABLE sessions (
    token_hash  text PRIMARY KEY,                    -- guardar el hash, no el token
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- Seguir a autores
CREATE TABLE follows (
    follower_id  bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followed_id  bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followed_id),
    CHECK (follower_id <> followed_id)
);
CREATE INDEX follows_followed_idx ON follows (followed_id);


-- ---------------------------------------------------------------------
-- CLASIFICACIÓN: GÉNEROS Y ETIQUETAS
-- ---------------------------------------------------------------------
CREATE TABLE genres (
    id     smallint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   text NOT NULL UNIQUE,                     -- Fantasía, Romance, Terror...
    slug   text NOT NULL UNIQUE
);

-- Etiquetas creadas por los autores. La identidad es slug (TagSlug en Go: sin mayúsculas,
-- sin tildes latinas, espacios y puntuación unificados en guiones); name es solo como se
-- muestra, tal como lo escribió quien la creó primero.
CREATE TABLE tags (
    id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),  -- maxTagRunes en Go
    slug   text NOT NULL UNIQUE
);
CREATE INDEX tags_slug_trgm ON tags USING gin (slug gin_trgm_ops);  -- sugerencias al escribir


-- ---------------------------------------------------------------------
-- HISTORIAS Y CAPÍTULOS
-- ---------------------------------------------------------------------
CREATE TABLE stories (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title            text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    slug             text NOT NULL,
    synopsis         text CHECK (char_length(synopsis) <= 4000),
    cover_url        text,
    kind             text NOT NULL DEFAULT 'original'
                     CHECK (kind IN ('original', 'fanfic')),  -- no es un género: un fanfic también los tiene
    language         text NOT NULL DEFAULT 'es',
    content_rating   text NOT NULL DEFAULT 'general'
                     CHECK (content_rating IN ('general', 'teen', 'mature')),
    -- Advertencias de contenido, de una lista cerrada (ContentWarnings en Go debe coincidir).
    -- 'none' (no aplica ninguna) va sola; 'declined' (el autor prefiere no advertir) se combina.
    warnings         text[] NOT NULL DEFAULT '{declined}'
                     CHECK (warnings <@ ARRAY['none', 'declined', 'graphic_violence',
                                              'major_character_death', 'sexual_violence']::text[])
                     CHECK (cardinality(warnings) >= 1)
                     CHECK (NOT ('none' = ANY (warnings) AND cardinality(warnings) > 1)),
    status           text NOT NULL DEFAULT 'draft'
                     CHECK (status IN ('draft', 'ongoing', 'completed', 'hiatus')),
    published_at     timestamptz,                    -- primera publicación
    last_chapter_at  timestamptz,                    -- para "actualizadas recientemente"
    reads_count      bigint NOT NULL DEFAULT 0,      -- contador desnormalizado
    -- Oculta por moderación: no sale en ningún listado público; solo la ven el autor y el equipo.
    hidden_at        timestamptz,
    hidden_reason    text,
    search           tsvector GENERATED ALWAYS AS (
                         setweight(to_tsvector('spanish', coalesce(title, '')), 'A') ||
                         setweight(to_tsvector('spanish', coalesce(synopsis, '')), 'B')
                     ) STORED,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (author_id, slug)
);
CREATE TRIGGER stories_updated_at BEFORE UPDATE ON stories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX stories_author_idx  ON stories (author_id);
CREATE INDEX stories_search_idx  ON stories USING gin (search);
CREATE INDEX stories_title_trgm  ON stories USING gin (title gin_trgm_ops);
CREATE INDEX stories_explore_idx ON stories (last_chapter_at DESC)
    WHERE status <> 'draft' AND hidden_at IS NULL;

-- Hasta 3 géneros por historia. position solo es el orden en que se muestran (lo elige el autor).
-- El CHECK y UNIQUE (story_id, position) limitan a 3 sin trigger; maxGenres en Go debe coincidir.
CREATE TABLE story_genres (
    story_id  bigint   NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    genre_id  smallint NOT NULL REFERENCES genres(id)  ON DELETE CASCADE,
    position  smallint NOT NULL CHECK (position BETWEEN 1 AND 3),
    PRIMARY KEY (story_id, genre_id),
    UNIQUE (story_id, position)
);
CREATE INDEX story_genres_genre_idx ON story_genres (genre_id);

-- Hasta 20 tags por historia, en el orden que elige el autor; maxTags en Go debe coincidir.
CREATE TABLE story_tags (
    story_id  bigint   NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    tag_id    bigint   NOT NULL REFERENCES tags(id)    ON DELETE CASCADE,
    position  smallint NOT NULL CHECK (position BETWEEN 1 AND 20),
    PRIMARY KEY (story_id, tag_id),
    UNIQUE (story_id, position)
);
CREATE INDEX story_tags_tag_idx ON story_tags (tag_id);

CREATE TABLE chapters (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    story_id      bigint NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    position      integer NOT NULL CHECK (position > 0),
    title         text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    content       text NOT NULL,                     -- Markdown o HTML ya sanitizado
    word_count    integer NOT NULL DEFAULT 0,
    is_published  boolean NOT NULL DEFAULT false,
    published_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- DEFERRABLE permite reordenar capítulos dentro de una transacción
    CONSTRAINT chapters_story_position_key UNIQUE (story_id, position)
        DEFERRABLE INITIALLY IMMEDIATE
);
CREATE TRIGGER chapters_updated_at BEFORE UPDATE ON chapters
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();


-- ---------------------------------------------------------------------
-- INTERACCIÓN: COMENTARIOS Y VALORACIONES
-- ---------------------------------------------------------------------
CREATE TABLE comments (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     bigint REFERENCES users(id) ON DELETE SET NULL,   -- se conserva el hilo si el usuario se va
    story_id    bigint NOT NULL REFERENCES stories(id)  ON DELETE CASCADE,
    chapter_id  bigint REFERENCES chapters(id)          ON DELETE CASCADE, -- NULL = comentario a la historia
    parent_id   bigint REFERENCES comments(id)          ON DELETE CASCADE, -- respuestas
    body        text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 5000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz                                       -- borrado suave
);
CREATE TRIGGER comments_updated_at BEFORE UPDATE ON comments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX comments_chapter_idx ON comments (chapter_id, created_at);
CREATE INDEX comments_story_idx   ON comments (story_id, created_at);
CREATE INDEX comments_parent_idx  ON comments (parent_id);

CREATE TABLE story_ratings (
    user_id     bigint NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    story_id    bigint NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    score       smallint NOT NULL CHECK (score BETWEEN 1 AND 5),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, story_id)
);
CREATE INDEX story_ratings_story_idx ON story_ratings (story_id);


-- ---------------------------------------------------------------------
-- BIBLIOTECA PERSONAL Y PROGRESO DE LECTURA
-- ---------------------------------------------------------------------
CREATE TABLE reading_lists (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    kind         text NOT NULL DEFAULT 'custom'
                 CHECK (kind IN ('favorites', 'library', 'custom')),
    is_public    boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);
-- Cada usuario tiene como máximo una lista "favorites" y una "library"
CREATE UNIQUE INDEX reading_lists_one_system_list
    ON reading_lists (user_id, kind) WHERE kind <> 'custom';

CREATE TABLE reading_list_items (
    list_id   bigint NOT NULL REFERENCES reading_lists(id) ON DELETE CASCADE,
    story_id  bigint NOT NULL REFERENCES stories(id)       ON DELETE CASCADE,
    added_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (list_id, story_id)
);
CREATE INDEX reading_list_items_story_idx ON reading_list_items (story_id);

CREATE TABLE reading_progress (
    user_id     bigint NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    story_id    bigint NOT NULL REFERENCES stories(id)  ON DELETE CASCADE,
    chapter_id  bigint REFERENCES chapters(id)          ON DELETE SET NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, story_id)
);


-- ---------------------------------------------------------------------
-- ACTUALIZACIONES / NOTIFICACIONES
-- ---------------------------------------------------------------------
CREATE TABLE notifications (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,   -- destinatario
    type        text   NOT NULL CHECK (type IN
                    ('new_chapter', 'new_story', 'new_comment', 'comment_reply',
                     'new_follower', 'new_rating')),
    actor_id    bigint REFERENCES users(id)    ON DELETE SET NULL,
    story_id    bigint REFERENCES stories(id)  ON DELETE CASCADE,
    chapter_id  bigint REFERENCES chapters(id) ON DELETE CASCADE,
    comment_id  bigint REFERENCES comments(id) ON DELETE CASCADE,
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx   ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;


-- ---------------------------------------------------------------------
-- MODERACIÓN (recomendado en plataformas con contenido de usuarios)
-- ---------------------------------------------------------------------
-- Los reportes de un capítulo llevan también story_id: la moderación agrupa por historia.
-- ReportReasons en Go debe coincidir con el CHECK de reason, y maxReportDetailsRunes con el de details.
CREATE TABLE reports (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reporter_id  bigint REFERENCES users(id)    ON DELETE SET NULL,
    story_id     bigint REFERENCES stories(id)  ON DELETE CASCADE,
    chapter_id   bigint REFERENCES chapters(id) ON DELETE CASCADE,
    comment_id   bigint REFERENCES comments(id) ON DELETE CASCADE,
    reason       text NOT NULL CHECK (reason IN
                     ('spam', 'plagiarism', 'harassment', 'inappropriate', 'other')),
    details      text CHECK (char_length(details) <= 1000),
    status       text NOT NULL DEFAULT 'open'
                 CHECK (status IN ('open', 'reviewing', 'resolved', 'dismissed')),
    reviewed_by  bigint REFERENCES users(id) ON DELETE SET NULL,  -- quién lo cerró
    reviewed_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(story_id, chapter_id, comment_id) >= 1)
);
CREATE INDEX reports_status_idx ON reports (status, created_at);
CREATE INDEX reports_story_idx  ON reports (story_id);

-- Registro de todo lo que hace el equipo de moderación. Las referencias son SET NULL para que
-- el registro sobreviva aunque se borre la historia o la cuenta; details guarda lo que haga
-- falta para entenderlo después (título, duración, rol nuevo…). ModerationActions en Go
-- debe coincidir con el CHECK de action.
CREATE TABLE moderation_actions (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    moderator_id    bigint REFERENCES users(id)   ON DELETE SET NULL,
    action          text NOT NULL CHECK (action IN
                        ('hide_story', 'unhide_story', 'dismiss_reports',
                         'suspend_user', 'unsuspend_user', 'change_role')),
    target_user_id  bigint REFERENCES users(id)   ON DELETE SET NULL,  -- la cuenta afectada (o el autor)
    story_id        bigint REFERENCES stories(id) ON DELETE SET NULL,
    reason          text,
    details         text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX moderation_actions_created_idx ON moderation_actions (created_at DESC);
CREATE INDEX moderation_actions_user_idx    ON moderation_actions (target_user_id, created_at DESC);
CREATE INDEX moderation_actions_story_idx   ON moderation_actions (story_id, created_at DESC);


-- ---------------------------------------------------------------------
-- DATOS INICIALES
-- ---------------------------------------------------------------------
INSERT INTO genres (name, slug) VALUES
    ('Fantasía', 'fantasia'), ('Romance', 'romance'), ('Ciencia ficción', 'ciencia-ficcion'),
    ('Terror', 'terror'), ('Misterio', 'misterio'), ('Aventura', 'aventura'),
    ('Drama', 'drama'), ('Humor', 'humor'), ('Poesía', 'poesia');
