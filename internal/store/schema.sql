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
