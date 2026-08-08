export interface GameTemplatePort {
  port: number;
  protocol: 'tcp' | 'udp';
  public: boolean;
  description?: string;
}

export interface GameTemplateVariable {
  name: string;
  env_variable: string;
  description?: string;
  default_value: string;
  user_viewable: boolean;
  user_editable: boolean;
  rules: string;
}

export interface GameTemplateResources {
  cpu: number;
  memory_mb: number;
  memory_max_mb?: number;
  disk_mb: number;
  cpu_shares?: number;
  io_weight?: number;
  swap_mb?: number;
}

export interface GameTemplateInstallScript {
  container: string;
  entrypoint: string;
  script: string;
}

export interface GameTemplateConfig {
  files?: Record<string, unknown>;
  startup: {
    done?: string;
  };
  stop: string;
  logs?: Record<string, unknown>;
}

export interface GameTemplate {
  $schema?: string;
  id: string;
  name: string;
  description: string;
  version: string;
  game: string;
  author?: string;
  update_url?: string;
  image: string;
  images?: Record<string, string>;
  startup: string;
  config: GameTemplateConfig;
  ports: GameTemplatePort[];
  env: GameTemplateVariable[];
  resources: GameTemplateResources;
  install_script: GameTemplateInstallScript;
  supported_platforms: ('docker' | 'podman')[];
  categories: string[];
  file_denylist?: string[];
  features?: string[];
}

export interface GameTemplateRegistryItem {
  id: string;
  name: string;
  description: string;
  game: string;
  version: string;
  categories: string[];
  tags: string[];
  source: string;
  author: string;
  updatedAt: string;
}

export interface GameTemplateCategoryInfo {
  name: string;
  description: string;
}

export interface GameTemplateRegistry {
  $schema?: string;
  version: string;
  registry: Record<string, GameTemplateRegistryItem>;
  categories: Record<string, GameTemplateCategoryInfo>;
}
