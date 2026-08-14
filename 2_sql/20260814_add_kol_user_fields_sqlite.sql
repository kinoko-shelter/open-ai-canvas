-- SQLite 手动迁移：users 表增加 KOL 用户 ID 映射字段。
-- 注意：SQLite ADD COLUMN 不支持 IF NOT EXISTS；如字段已存在，不要重复执行本文件。
ALTER TABLE users ADD COLUMN kol_user_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_users_kol_user_id ON users(kol_user_id);
