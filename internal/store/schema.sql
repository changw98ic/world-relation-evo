CREATE TABLE IF NOT EXISTS schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS worlds (
    id text PRIMARY KEY,
    tick bigint NOT NULL,
    state jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS world_entities (
    world_id text NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    kind text NOT NULL,
    entity_id text NOT NULL,
    ordinal bigint NOT NULL,
    state jsonb NOT NULL,
    PRIMARY KEY (world_id, kind, entity_id)
);
CREATE INDEX IF NOT EXISTS world_entities_order_idx ON world_entities(world_id, kind, ordinal);
CREATE INDEX IF NOT EXISTS world_entities_wake_idx
    ON world_entities(world_id, ((state #>> '{next_wake,tick}')::bigint), entity_id)
    WHERE kind = 'actor' AND state ? 'next_wake';

CREATE TABLE IF NOT EXISTS world_events (
    world_id text NOT NULL REFERENCES worlds(id),
    id bigint NOT NULL,
    tick bigint NOT NULL,
    actor_id text NOT NULL,
    target_id text NOT NULL,
    kind text NOT NULL,
    action jsonb NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY (world_id, id)
);
CREATE INDEX IF NOT EXISTS world_events_tick_idx ON world_events(world_id, tick, id);
CREATE INDEX IF NOT EXISTS world_events_actor_idx ON world_events(world_id, actor_id, id);

CREATE TABLE IF NOT EXISTS world_checkpoints (
    world_id text NOT NULL REFERENCES worlds(id),
    tick bigint NOT NULL,
    state jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (world_id, tick)
);

CREATE TABLE IF NOT EXISTS world_decision_logs (
    world_id text NOT NULL REFERENCES worlds(id),
    tick bigint NOT NULL,
    decisions jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (world_id, tick)
);


DO $$
BEGIN
    -- Serialize constraint discovery and creation across concurrent initializers.
    LOCK TABLE schema_migrations IN SHARE ROW EXCLUSIVE MODE;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE contype = 'f' AND conname = 'world_checkpoints_world_id_fkey'
          AND conrelid = 'world_checkpoints'::regclass
    ) THEN
        ALTER TABLE world_checkpoints ADD CONSTRAINT world_checkpoints_world_id_fkey
            FOREIGN KEY (world_id) REFERENCES worlds(id) NOT VALID;
    END IF;
    ALTER TABLE world_checkpoints VALIDATE CONSTRAINT world_checkpoints_world_id_fkey;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE contype = 'f' AND conname = 'world_decision_logs_world_id_fkey'
          AND conrelid = 'world_decision_logs'::regclass
    ) THEN
        ALTER TABLE world_decision_logs ADD CONSTRAINT world_decision_logs_world_id_fkey
            FOREIGN KEY (world_id) REFERENCES worlds(id) NOT VALID;
    END IF;
    ALTER TABLE world_decision_logs VALIDATE CONSTRAINT world_decision_logs_world_id_fkey;
    IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 2) THEN
        -- Some builds wrote only the action map. The reader uses the complete tick log.
        UPDATE world_decision_logs
        SET decisions = jsonb_build_object('tick', tick, 'decisions', decisions)
        WHERE jsonb_typeof(decisions) IN ('object', 'null')
          AND (jsonb_typeof(decisions -> 'tick') IS DISTINCT FROM 'number' OR NOT decisions ? 'decisions');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 3) THEN
        LOCK TABLE worlds IN SHARE ROW EXCLUSIVE MODE;
        INSERT INTO world_entities (world_id, kind, entity_id, ordinal, state)
        SELECT w.id, sources.kind, entity.state->>'id', entity.ordinal, entity.state
        FROM worlds AS w
        CROSS JOIN LATERAL (VALUES ('actor', w.state->'actors'), ('cohort', w.state->'cohorts')) sources(kind, entities)
        CROSS JOIN LATERAL jsonb_array_elements(
            CASE WHEN jsonb_typeof(sources.entities) = 'array' THEN sources.entities ELSE '[]'::jsonb END
        ) WITH ORDINALITY entity(state, ordinal);
        UPDATE worlds
        SET state = CASE
            WHEN jsonb_typeof(state->'actors') = 'array' THEN jsonb_set(state - 'cohorts', '{actors}', '[]')
            ELSE state - 'cohorts'
        END
        WHERE jsonb_typeof(state->'actors') = 'array' OR state ? 'cohorts';
        INSERT INTO schema_migrations (version) VALUES (3);
    END IF;
END $$;

INSERT INTO schema_migrations (version) VALUES (1), (2) ON CONFLICT DO NOTHING;
