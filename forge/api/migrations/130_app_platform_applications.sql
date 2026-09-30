-- applications and app_services tables are defined in migration 100_z_app_platform_applications.sql
-- This migration adds missing constraints.

DO $$ BEGIN ALTER TABLE applications ADD CONSTRAINT applications_server_fk FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE SET NULL; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN ALTER TABLE applications ADD CONSTRAINT applications_deployment_fk FOREIGN KEY (current_deployment_id) REFERENCES deployments(id) ON DELETE SET NULL; EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN ALTER TABLE applications ADD CONSTRAINT fk_applications_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN ALTER TABLE app_services ADD CONSTRAINT fk_app_services_app FOREIGN KEY (app_id) REFERENCES applications(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_applications_org_id ON applications (org_id);
CREATE INDEX IF NOT EXISTS idx_applications_project_id ON applications (project_id);
CREATE INDEX IF NOT EXISTS idx_applications_environment_id ON applications (environment_id);
CREATE INDEX IF NOT EXISTS idx_applications_server_id ON applications (server_id);
CREATE INDEX IF NOT EXISTS idx_applications_source_type ON applications (source_type);
CREATE INDEX IF NOT EXISTS idx_applications_org_created ON applications (org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_app_services_app_id ON app_services (app_id);
