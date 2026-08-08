package appstore

import (
	"context"
	"log/slog"

	"gamepanel/forge/internal/store"
)

type seedApp struct {
	Key            string
	Name           string
	ShortDesc      string
	Description    string
	Icon           string
	Category       string
	Tags           []string
	Version        string
	ComposeContent string
	Params         string
	MinMemoryMB    int
	MinDiskMB      int
	Maintainer     string
	SourceURL      string
}

var seedApps = []seedApp{
	{
		Key: "nginx", Name: "Nginx",
		ShortDesc:   "High-performance web server and reverse proxy",
		Description: "Nginx is a popular open-source web server that can also be used as a reverse proxy, load balancer, and HTTP cache.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/nginx.svg",
		Category:    "web-server",
		Tags:        []string{"web", "proxy", "http", "reverse-proxy"},
		Version:     "latest",
		ComposeContent: `version: '3.8'
services:
  nginx:
    image: nginx:${NGINX_TAG:-alpine}
    container_name: nginx-${APP_NAME}
    ports:
      - "${NGINX_PORT:-80}:80"
    volumes:
      - nginx-${APP_NAME}-html:/usr/share/nginx/html:ro
    restart: unless-stopped
volumes:
  nginx-${APP_NAME}-html:`,
		Params: `{"NGINX_TAG":{"label":"Nginx Image Tag","type":"string","default":"alpine","description":"Nginx Docker image tag"},"NGINX_PORT":{"label":"HTTP Port","type":"number","default":80,"description":"External HTTP port"}}`,
		MinMemoryMB: 64, MinDiskMB: 256, Maintainer: "Forge Team", SourceURL: "https://hub.docker.com/_/nginx",
	},
	{
		Key: "postgres", Name: "PostgreSQL",
		ShortDesc:   "Advanced open-source relational database",
		Description: "PostgreSQL is a powerful, open-source object-relational database system with a strong reputation for reliability and performance.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/postgresql.svg",
		Category:    "database",
		Tags:        []string{"database", "sql", "relational"},
		Version:     "16",
		ComposeContent: `version: '3.8'
services:
  postgres:
    image: postgres:${POSTGRES_VERSION:-16}
    container_name: postgres-${APP_NAME}
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-app}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-changeme}
      POSTGRES_DB: ${POSTGRES_DB:-appdb}
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
    volumes:
      - postgres-${APP_NAME}-data:/var/lib/postgresql/data
    restart: unless-stopped
volumes:
  postgres-${APP_NAME}-data:`,
		Params: `{"POSTGRES_VERSION":{"label":"PostgreSQL Version","type":"string","default":"16","description":"PostgreSQL major version"},"POSTGRES_PORT":{"label":"Database Port","type":"number","default":5432,"description":"External port"},"POSTGRES_USER":{"label":"Database User","type":"string","default":"app","description":"Default database user"},"POSTGRES_PASSWORD":{"label":"Database Password","type":"string","default":"changeme","description":"Password for the database user"},"POSTGRES_DB":{"label":"Database Name","type":"string","default":"appdb","description":"Default database name"}}`,
		MinMemoryMB: 256, MinDiskMB: 1024, Maintainer: "Forge Team", SourceURL: "https://hub.docker.com/_/postgres",
	},
	{
		Key: "redis", Name: "Redis",
		ShortDesc:   "In-memory data structure store, used as cache and message broker",
		Description: "Redis is an open-source, in-memory data structure store used as a database, cache, and message broker.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/redis.svg",
		Category:    "cache",
		Tags:        []string{"cache", "in-memory", "key-value"},
		Version:     "7",
		ComposeContent: `version: '3.8'
services:
  redis:
    image: redis:${REDIS_VERSION:-7-alpine}
    container_name: redis-${APP_NAME}
    ports:
      - "${REDIS_PORT:-6379}:6379"
    volumes:
      - redis-${APP_NAME}-data:/data
    command: redis-server --appendonly yes --requirepass ${REDIS_PASSWORD:-}
    restart: unless-stopped
volumes:
  redis-${APP_NAME}-data:`,
		Params: `{"REDIS_VERSION":{"label":"Redis Version","type":"string","default":"7-alpine","description":"Redis Docker image tag"},"REDIS_PORT":{"label":"Redis Port","type":"number","default":6379,"description":"External port"},"REDIS_PASSWORD":{"label":"Redis Password","type":"string","default":"","description":"Password (leave empty for no auth)"}}`,
		MinMemoryMB: 64, MinDiskMB: 128, Maintainer: "Forge Team", SourceURL: "https://hub.docker.com/_/redis",
	},
	{
		Key: "mongo", Name: "MongoDB",
		ShortDesc:   "NoSQL document database for modern applications",
		Description: "MongoDB is a source-available, cross-platform, document-oriented database program using JSON-like documents.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/mongodb.svg",
		Category:    "database",
		Tags:        []string{"database", "nosql", "document"},
		Version:     "7",
		ComposeContent: `version: '3.8'
services:
  mongo:
    image: mongo:${MONGO_VERSION:-7}
    container_name: mongo-${APP_NAME}
    environment:
      MONGO_INITDB_ROOT_USERNAME: ${MONGO_USER:-admin}
      MONGO_INITDB_ROOT_PASSWORD: ${MONGO_PASSWORD:-changeme}
      MONGO_INITDB_DATABASE: ${MONGO_DB:-appdb}
    ports:
      - "${MONGO_PORT:-27017}:27017"
    volumes:
      - mongo-${APP_NAME}-data:/data/db
    restart: unless-stopped
volumes:
  mongo-${APP_NAME}-data:`,
		Params: `{"MONGO_VERSION":{"label":"MongoDB Version","type":"string","default":"7","description":"MongoDB major version"},"MONGO_PORT":{"label":"MongoDB Port","type":"number","default":27017,"description":"External port"},"MONGO_USER":{"label":"Root Username","type":"string","default":"admin","description":"Root username"},"MONGO_PASSWORD":{"label":"Root Password","type":"string","default":"changeme","description":"Root password"},"MONGO_DB":{"label":"Initial Database","type":"string","default":"appdb","description":"Initial database"}}`,
		MinMemoryMB: 256, MinDiskMB: 512, Maintainer: "Forge Team", SourceURL: "https://hub.docker.com/_/mongo",
	},
	{
		Key: "mariadb", Name: "MariaDB",
		ShortDesc:   "Open-source relational database, a fork of MySQL",
		Description: "MariaDB is a community-developed fork of the MySQL relational database management system.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/mariadb.svg",
		Category:    "database",
		Tags:        []string{"database", "sql", "mysql-compatible"},
		Version:     "11",
		ComposeContent: `version: '3.8'
services:
  mariadb:
    image: mariadb:${MARIADB_VERSION:-11}
    container_name: mariadb-${APP_NAME}
    environment:
      MYSQL_ROOT_PASSWORD: ${MARIADB_ROOT_PASSWORD:-changeme}
      MYSQL_DATABASE: ${MARIADB_DB:-appdb}
      MYSQL_USER: ${MARIADB_USER:-app}
      MYSQL_PASSWORD: ${MARIADB_PASSWORD:-changeme}
    ports:
      - "${MARIADB_PORT:-3306}:3306"
    volumes:
      - mariadb-${APP_NAME}-data:/var/lib/mysql
    restart: unless-stopped
volumes:
  mariadb-${APP_NAME}-data:`,
		Params: `{"MARIADB_VERSION":{"label":"MariaDB Version","type":"string","default":"11","description":"MariaDB major version"},"MARIADB_PORT":{"label":"MariaDB Port","type":"number","default":3306,"description":"External port"},"MARIADB_ROOT_PASSWORD":{"label":"Root Password","type":"string","default":"changeme","description":"Root password"},"MARIADB_USER":{"label":"App User","type":"string","default":"app","description":"Application user"},"MARIADB_PASSWORD":{"label":"App Password","type":"string","default":"changeme","description":"Application password"},"MARIADB_DB":{"label":"Database","type":"string","default":"appdb","description":"Database name"}}`,
		MinMemoryMB: 256, MinDiskMB: 1024, Maintainer: "Forge Team", SourceURL: "https://hub.docker.com/_/mariadb",
	},
	{
		Key: "portainer", Name: "Portainer",
		ShortDesc:   "Lightweight container management UI",
		Description: "Portainer is a lightweight management UI which allows you to easily manage your Docker environments.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/portainer.svg",
		Category:    "management",
		Tags:        []string{"docker", "management", "ui"},
		Version:     "latest",
		ComposeContent: `version: '3.8'
services:
  portainer:
    image: portainer/portainer-ce:${PORTAINER_VERSION:-latest}
    container_name: portainer-${APP_NAME}
    ports:
      - "${PORTAINER_PORT:-9000}:9000"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - portainer-${APP_NAME}-data:/data
    restart: unless-stopped
volumes:
  portainer-${APP_NAME}-data:`,
		Params: `{"PORTAINER_VERSION":{"label":"Portainer Version","type":"string","default":"latest","description":"Portainer image tag"},"PORTAINER_PORT":{"label":"Web UI Port","type":"number","default":9000,"description":"External port for the Portainer web UI"}}`,
		MinMemoryMB: 128, MinDiskMB: 256, Maintainer: "Forge Team", SourceURL: "https://docs.portainer.io/",
	},
	{
		Key: "traefik", Name: "Traefik",
		ShortDesc:   "Cloud-native application proxy and reverse proxy",
		Description: "Traefik is a leading modern reverse proxy and load balancer that makes deploying microservices easy.",
		Icon:        "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/traefikproxy.svg",
		Category:    "proxy",
		Tags:        []string{"proxy", "load-balancer", "reverse-proxy", "tls"},
		Version:     "latest",
		ComposeContent: `version: '3.8'
services:
  traefik:
    image: traefik:${TRAEFIK_VERSION:-latest}
    container_name: traefik-${APP_NAME}
    ports:
      - "${TRAEFIK_HTTP_PORT:-80}:80"
      - "${TRAEFIK_HTTPS_PORT:-443}:443"
      - "${TRAEFIK_DASHBOARD_PORT:-8080}:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - traefik-${APP_NAME}-config:/etc/traefik
    command:
      - "--api.insecure=true"
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
    restart: unless-stopped
volumes:
  traefik-${APP_NAME}-config:`,
		Params: `{"TRAEFIK_VERSION":{"label":"Traefik Version","type":"string","default":"latest","description":"Traefik image tag"},"TRAEFIK_HTTP_PORT":{"label":"HTTP Port","type":"number","default":80,"description":"External HTTP port"},"TRAEFIK_HTTPS_PORT":{"label":"HTTPS Port","type":"number","default":443,"description":"External HTTPS port"},"TRAEFIK_DASHBOARD_PORT":{"label":"Dashboard Port","type":"number","default":8080,"description":"Dashboard UI port"}}`,
		MinMemoryMB: 128, MinDiskMB: 256, Maintainer: "Forge Team", SourceURL: "https://hub.docker.com/_/traefik",
	},
}

func (s *Service) SeedDefaultApps(ctx context.Context) error {
	for _, app := range seedApps {
		paramsBytes := []byte(app.Params)
		err := s.store.UpsertAppStoreApp(ctx, &store.AppStoreApp{
			Key:            app.Key,
			Name:           app.Name,
			ShortDesc:      app.ShortDesc,
			Description:    app.Description,
			Icon:           app.Icon,
			Category:       app.Category,
			Tags:           app.Tags,
			Version:        app.Version,
			ComposeContent: app.ComposeContent,
			Params:         paramsBytes,
			MinMemoryMB:    app.MinMemoryMB,
			MinDiskMB:      app.MinDiskMB,
			Maintainer:     app.Maintainer,
			SourceURL:      app.SourceURL,
		})
		if err != nil {
			slog.Warn("seed app store app", "key", app.Key, "error", err)
		}
	}
	return nil
}
