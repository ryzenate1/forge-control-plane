#!/bin/bash

# GamePanel Diagnostic Script
# Checks all services and reports status

echo "========================================"
echo "🔍 GamePanel System Diagnostic"
echo "========================================"
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check API
echo -n "API (port 8080): "
if curl -s http://localhost:8080/api/v1/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Running${NC}"
else
    echo -e "${RED}✗ Not responding${NC}"
fi

# Check Daemon
echo -n "Daemon (port 9090): "
if curl -s http://localhost:9090/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Running${NC}"
else
    echo -e "${RED}✗ Not responding${NC}"
fi

# Check Frontend
echo -n "Frontend (port 3000): "
if curl -s http://localhost:3000 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Running${NC}"
else
    echo -e "${RED}✗ Not responding${NC}"
fi

# Check PostgreSQL
echo -n "PostgreSQL: "
if docker exec docker-postgres-1 pg_isready -U gamepanel > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Running${NC}"
else
    echo -e "${RED}✗ Not running${NC}"
fi

# Check Redis
echo -n "Redis: "
if docker exec docker-redis-1 redis-cli ping > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Running${NC}"
else
    echo -e "${RED}✗ Not running${NC}"
fi

echo ""
echo "----------------------------------------"
echo "📊 Testing API Authentication"
echo "----------------------------------------"

# Test login
echo -n "Login endpoint: "
LOGIN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"admin123"}')

if echo "$LOGIN_RESPONSE" | grep -q "token"; then
    echo -e "${GREEN}✓ Working${NC}"
    TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"token":"[^"]*' | cut -d'"' -f4)
    echo "   Token: ${TOKEN:0:50}..."
    
    # Test authenticated endpoint
    echo -n "Authenticated request: "
    ME_RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/auth/me)
    if echo "$ME_RESPONSE" | grep -q "admin@example.com"; then
        echo -e "${GREEN}✓ Working${NC}"
    else
        echo -e "${RED}✗ Failed${NC}"
        echo "   Response: $ME_RESPONSE"
    fi
else
    echo -e "${RED}✗ Failed${NC}"
    echo "   Response: $LOGIN_RESPONSE"
fi

echo ""
echo "----------------------------------------"
echo "🗄️  Database Status"
echo "----------------------------------------"

# Count migrations
MIGRATIONS=$(docker exec docker-postgres-1 psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM schema_migrations" 2>/dev/null | tr -d ' ')
if [ -n "$MIGRATIONS" ]; then
    echo "Applied migrations: ${GREEN}$MIGRATIONS${NC}"
else
    echo -e "Migrations: ${RED}Unable to query${NC}"
fi

# Count servers
SERVERS=$(docker exec docker-postgres-1 psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM servers" 2>/dev/null | tr -d ' ')
if [ -n "$SERVERS" ]; then
    echo "Servers in database: ${GREEN}$SERVERS${NC}"
fi

# Count nodes
NODES=$(docker exec docker-postgres-1 psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM nodes" 2>/dev/null | tr -d ' ')
if [ -n "$NODES" ]; then
    echo "Nodes in database: ${GREEN}$NODES${NC}"
fi

# Count allocations
ALLOCATIONS=$(docker exec docker-postgres-1 psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM allocations" 2>/dev/null | tr -d ' ')
if [ -n "$ALLOCATIONS" ]; then
    echo "Allocations in database: ${GREEN}$ALLOCATIONS${NC}"
fi

echo ""
echo "----------------------------------------"
echo "📝 Recent Logs"
echo "----------------------------------------"

echo ""
echo "Last 5 API log entries:"
tail -5 .dev-logs/api.log 2>/dev/null || echo "No API logs found"

echo ""
echo "Last 5 API errors:"
tail -5 .dev-logs/api.err.log 2>/dev/null || echo "No API errors"

echo ""
echo "Last 5 Daemon errors:"
tail -5 .dev-logs/daemon.err.log 2>/dev/null || echo "No Daemon errors"

echo ""
echo "========================================"
echo "✨ Diagnostic Complete"
echo "========================================"
echo ""
echo "Next steps:"
echo "1. If all services are green, open http://localhost:3000"
echo "2. Open browser DevTools (F12) → Console tab"
echo "3. Login with admin@example.com / admin123"
echo "4. Check for errors in browser console"
echo "5. Report any red errors you see"
echo ""
echo "For more details, see: INTEGRATION_FIX_PLAN.md"
echo ""
