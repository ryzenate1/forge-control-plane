package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"gamepanel/forge/internal/store"
)

// kindForEntry maps a catalog key to its runtime kind. The db_containers
// runtime uses its own engine names (postgresql vs postgres), while queue and
// cache entries keep their catalog key as the runtime kind.
func kindForEntry(key string) string {
	switch key {
	case "postgres":
		return "postgresql"
	case "redis-queue":
		return "redis"
	default:
		return key
	}
}

// engineForEntry maps a catalog key to the engine string accepted by
// store.ValidateDBEngine / DBContainerService.
func engineForEntry(key string) string {
	switch key {
	case "postgres":
		return "postgresql"
	default:
		return key
	}
}

// isManagedDBKind reports whether the entry provisions through the
// db_containers / managed databases runtime (works only for the engines the
// beacon can containerize). Everything else uses the baked compose templates.
func isManagedDBKind(key string) bool {
	switch key {
	case "postgres", "mysql", "mariadb", "redis", "mongodb":
		return true
	default:
		return false
	}
}

// defaultPortFor returns the published port for each catalog runtime.
func defaultPortFor(key string) int {
	switch key {
	case "postgres":
		return 5432
	case "mysql", "mariadb":
		return 3306
	case "redis", "valkey", "redis-queue":
		return 6379
	case "mongodb":
		return 27017
	case "rabbitmq":
		return 5672
	case "nats":
		return 4222
	case "memcached":
		return 11211
	case "clickhouse":
		return 8123
	default:
		return 0
	}
}

// connectionStringFor builds the URL form of a runtime's connection for a
// catalog kind. user/password/database may be empty for unauthenticated
// runtimes (memcached, nats).
func connectionStringFor(key, username, password, host string, port int, version string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	switch key {
	case "postgres":
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/app?sslmode=disable", url.QueryEscape(username), url.QueryEscape(password), host, port)
	case "mysql":
		return fmt.Sprintf("mysql://%s:%s@%s:%d/app", url.QueryEscape(username), url.QueryEscape(password), host, port)
	case "mariadb":
		return fmt.Sprintf("mariadb://%s:%s@%s:%d/app", url.QueryEscape(username), url.QueryEscape(password), host, port)
	case "mongodb":
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/app", url.QueryEscape(username), url.QueryEscape(password), host, port)
	case "redis", "valkey", "redis-queue":
		if password != "" {
			return fmt.Sprintf("redis://:%s@%s:%d/0", url.QueryEscape(password), host, port)
		}
		return fmt.Sprintf("redis://%s:%d/0", host, port)
	case "rabbitmq":
		return fmt.Sprintf("amqp://%s:%s@%s:%d/", url.QueryEscape(username), url.QueryEscape(password), host, port)
	case "nats":
		return fmt.Sprintf("nats://%s:%d", host, port)
	case "memcached":
		return fmt.Sprintf("memcached://%s:%d", host, port)
	case "clickhouse":
		return fmt.Sprintf("clickhouse://%s:%s@%s:%d/app", url.QueryEscape(username), url.QueryEscape(password), host, port)
	default:
		return ""
	}
}

// templateName builds a stable slug compose stack name for an entry.
func templateName(entry *store.CatalogEntry, version, nodeID string) string {
	slug := strings.NewReplacer(".", "-", "_", "-").Replace(version)
	return fmt.Sprintf("catalog-%s-%s-%s", entry.Key, slug, shortID(nodeID))
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// composeTemplateFor returns the baked compose template for a queue/cache
// entry, interpolating the requested version and a placeholder password that
// provisionCompose replaces with the generated secret.
func composeTemplateFor(entry *store.CatalogEntry, version, password string) string {
	key := entry.Key
	switch key {
	case "postgres":
		return tpl(fmt.Sprintf(`services:
  db:
    image: postgres:%s
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: %s
    ports:
      - "5432:5432"
    volumes:
      - data:/var/lib/postgresql/data
volumes:
  data:`, version, password))
	case "mysql", "mariadb":
		image := "mysql"
		if key == "mariadb" {
			image = "mariadb"
		}
		return tpl(fmt.Sprintf(`services:
  db:
    image: %s:%s
    environment:
      MYSQL_DATABASE: app
      MYSQL_USER: app
      MYSQL_PASSWORD: %s
      MYSQL_ROOT_PASSWORD: %s
    ports:
      - "3306:3306"
    volumes:
      - data:/var/lib/mysql
volumes:
  data:`, image, version, password, password))
	case "redis", "redis-queue":
		return tpl(fmt.Sprintf(`services:
  redis:
    image: redis:%s
    command: ["redis-server", "--requirepass", "%s"]
    ports:
      - "6379:6379"
    volumes:
      - data:/data
volumes:
  data:`, version, password))
	case "valkey":
		return tpl(fmt.Sprintf(`services:
  valkey:
    image: valkey/valkey:%s
    command: ["valkey-server", "--requirepass", "%s"]
    ports:
      - "6379:6379"
    volumes:
      - data:/data
volumes:
  data:`, version, password))
	case "mongodb":
		return tpl(fmt.Sprintf(`services:
  mongo:
    image: mongo:%s
    environment:
      MONGO_INITDB_DATABASE: app
      MONGO_INITDB_ROOT_USERNAME: app
      MONGO_INITDB_ROOT_PASSWORD: %s
    ports:
      - "27017:27017"
    volumes:
      - data:/data/db
volumes:
  data:`, version, password))
	case "rabbitmq":
		return tpl(fmt.Sprintf(`services:
  rabbitmq:
    image: rabbitmq:%s
    environment:
      RABBITMQ_DEFAULT_USER: app
      RABBITMQ_DEFAULT_PASS: %s
    ports:
      - "5672:5672"
      - "15672:15672"
    volumes:
      - data:/var/lib/rabbitmq
volumes:
  data:`, version, password))
	case "clickhouse":
		return tpl(fmt.Sprintf(`services:
  clickhouse:
    image: clickhouse/clickhouse-server:%s
    ports:
      - "8123:8123"
      - "9000:9000"
    volumes:
      - data:/var/lib/clickhouse
volumes:
  data:`, version))
	case "nats":
		return tpl(`services:
  nats:
    image: nats:2
    command: ["-js", "-m", "8222"]
    ports:
      - "4222:4222"
      - "8222:8222"`)
	case "memcached":
		return tpl(`services:
  memcached:
    image: memcached:2
    ports:
      - "11211:11211"`)
	default:
		return entry.DisplayName + "-template"
	}
}

// tpl normalizes indentation so Go compiler unit tests and the template match
// byte-for-byte.
func tpl(s string) string { return strings.TrimSpace(s) + "\n" }

// randomPassword returns a URL-safe generated secret for a new instance.
func randomPassword(length int) string {
	if length <= 0 {
		length = 24
	}
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "changeme"
	}
	return hex.EncodeToString(b)[:length]
}
