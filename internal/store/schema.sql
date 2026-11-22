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
        BEGIN
            ALTER TABLE world_checkpoints VALIDATE CONSTRAINT world_checkpoints_world_id_fkey;
        EXCEPTION WHEN foreign_key_violation THEN
            RAISE WARNING 'world_checkpoints contains historical rows without a world; new writes are subject to its foreign key';
        END;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE contype = 'f' AND conname = 'world_decision_logs_world_id_fkey'
          AND conrelid = 'world_decision_logs'::regclass
    ) THEN
        ALTER TABLE world_decision_logs ADD CONSTRAINT world_decision_logs_world_id_fkey
            FOREIGN KEY (world_id) REFERENCES worlds(id) NOT VALID;
        BEGIN
            ALTER TABLE world_decision_logs VALIDATE CONSTRAINT world_decision_logs_world_id_fkey;
        EXCEPTION WHEN foreign_key_violation THEN
            RAISE WARNING 'world_decision_logs contains historical rows without a world; new writes are subject to its foreign key';
        END;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 2) THEN
        -- Some builds wrote only the action map. The reader uses the complete tick log.
        UPDATE world_decision_logs
        SET decisions = jsonb_build_object('tick', tick, 'decisions', decisions)
        WHERE jsonb_typeof(decisions) IN ('object', 'null')
          AND (jsonb_typeof(decisions -> 'tick') IS DISTINCT FROM 'number' OR NOT decisions ? 'decisions');
    END IF;
END $$;

INSERT INTO schema_migrations (version) VALUES (1), (2) ON CONFLICT DO NOTHING;
