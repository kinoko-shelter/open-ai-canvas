-- PostgreSQL 手动迁移：users 表增加 KOL 用户 ID 映射字段。
ALTER TABLE users ADD COLUMN IF NOT EXISTS kol_user_id varchar(160) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_users_kol_user_id ON users(kol_user_id);
