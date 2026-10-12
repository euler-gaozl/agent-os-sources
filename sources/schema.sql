CREATE TABLE IF NOT EXISTS source_blobs(hash text PRIMARY KEY,body bytea NOT NULL);
 CREATE TABLE IF NOT EXISTS source_captures(id text PRIMARY KEY,url text NOT NULL,hash text NOT NULL REFERENCES source_blobs(hash),media_type text NOT NULL,title text NOT NULL,content text NOT NULL,links jsonb NOT NULL,fetched_at timestamptz NOT NULL DEFAULT now());
 CREATE INDEX IF NOT EXISTS source_captures_url ON source_captures(url,fetched_at DESC);
