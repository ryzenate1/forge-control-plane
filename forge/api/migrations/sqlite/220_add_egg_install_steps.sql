-- SQLite dialect override for 220_add_egg_install_steps.sql
ALTER TABLE eggs ADD COLUMN install_steps TEXT NOT NULL DEFAULT '[]';
