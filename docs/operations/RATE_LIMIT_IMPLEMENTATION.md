# Rate Limit Configuration Implementation

## Summary

Implemented complete rate limit configuration management for the Game Panel, including:
- Backend API endpoints (GET/PUT) for rate limit settings
- Database storage with migration
- Admin UI component with form controls
- File download button component with signed URL support

## Files Created

### Backend (forge/api)

1. **migrations/058_rate_limit_settings.sql**
   - Creates `panel_rate_limit_settings` table
   - Stores settings as JSON blob

2. **internal/store/store_rate_limit_settings.go**
   - `RateLimitSettings` struct with 11 configurable fields
   - `DefaultRateLimitSettings()` function
   - `GetRateLimitSettings(ctx)` - retrieves from database
   - `UpdateRateLimitSettings(ctx, settings)` - persists to database

3. **internal/http/handlers_rate_limit_config.go**
   - `GET /admin/settings/rate-limits` - fetch current settings
   - `PUT /admin/settings/rate-limits` - update settings
   - Admin-only access with `requireRole("admin")` and `requireAdminScope("settings.write")`

4. **internal/http/handlers_rate_limit_config_test.go**
   - 4 test cases covering defaults, no-store scenarios, and validation
   - All tests passing ✓

### Frontend (forge/web)

5. **lib/api/rateLimits.ts**
   - TypeScript API client for rate limit endpoints
   - `RateLimitSettings` type definition
   - `getRateLimitSettings()` and `updateRateLimitSettings()` functions
   - Exported via `lib/api/index.ts`

6. **components/admin/AdminRateLimitSettings.tsx**
   - Complete admin UI component
   - Form with all 11 configuration fields
   - Toggle switches for boolean settings
   - Number inputs for numeric settings
   - Save button with loading state
   - Toast notifications for success/error

7. **components/server/file-download-button.tsx**
   - Download button component for file browser
   - Generates signed URLs via `GET /servers/:id/files/download-url`
   - Triggers browser download with proper filename
   - Loading state during URL generation

### Modified Files

8. **internal/http/server.go**
   - Added route registration: `registerRateLimitSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)`

## Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `authRequestsPerMinute` | number | 5 | Auth endpoint rate limit |
| `mutationRequestsPerMinute` | number | 30 | Mutation endpoint rate limit |
| `readRequestsPerMinute` | number | 120 | Read endpoint rate limit |
| `loginRateLimitEnabled` | boolean | true | Enable login rate limiting |
| `loginAttemptThreshold` | number | 5 | Failed attempts before lockout |
| `accountLockoutMinutes` | number | 15 | Lockout duration |
| `signedUrlExpiryMinutes` | number | 5 | Signed URL expiration |
| `maxWebSocketsPerServer` | number | 30 | WebSocket connection limit |
| `consoleThrottleEnabled` | boolean | false | Enable console throttling |
| `consoleThrottleLines` | number | 2000 | Lines per period |
| `consoleThrottlePeriodMs` | number | 100 | Throttle period |

## API Endpoints

### GET /admin/settings/rate-limits
Returns current rate limit configuration.

**Response:**
```json
{
  "authRequestsPerMinute": 5,
  "mutationRequestsPerMinute": 30,
  "readRequestsPerMinute": 120,
  "loginRateLimitEnabled": true,
  "loginAttemptThreshold": 5,
  "accountLockoutMinutes": 15,
  "signedUrlExpiryMinutes": 5,
  "maxWebSocketsPerServer": 30,
  "consoleThrottleEnabled": false,
  "consoleThrottleLines": 2000,
  "consoleThrottlePeriodMs": 100
}
```

### PUT /admin/settings/rate-limits
Updates rate limit configuration (admin only).

**Request Body:** Same as response format above.

**Response:** Updated settings object.

## Verification

### Backend Tests
```bash
cd forge/api
go build ./...           # ✓ Build successful
go test ./internal/http/... -v -run "RateLimit"
# TestRateLimitSettings_Defaults              PASS
# TestRateLimitSettings_NoStore_ReturnsDefaults PASS
# TestRateLimitSettings_NoStore_UpdateFails   PASS
# TestRateLimitSettings_InvalidBody           PASS
```

### Frontend Type Check
```bash
cd forge/web
npx tsc --noEmit  # ✓ No errors in new files
```

## Integration Notes

### Wiring to Existing Middleware

The rate limit settings are now stored in the database but need to be wired to the existing rate limiting middleware in `middleware_ratelimit.go`. 

**Current state:**
- Middleware uses hardcoded values in `GetRateLimitForEndpoint()`
- Settings are stored and retrievable via API
- UI allows configuration changes

**Next steps (recommended):**
1. Add `GetActiveRateLimitSettings()` helper to store package
2. Modify `GetRateLimitForEndpoint()` to query database
3. Add caching layer to avoid DB queries on every request
4. Consider in-memory cache with 30s TTL for performance

### File Download Integration

The `file-download-button.tsx` component is ready to use in the file browser:

```tsx
import { FileDownloadButton } from '@/components/server/file-download-button';

<FileDownloadButton 
  serverId={server.id}
  filePath={file.path}
  fileName={file.name}
/>
```

The component calls the existing `GET /servers/:id/files/download-url` endpoint which was already implemented in the backend.

## Security

- All endpoints require admin role (`requireRole("admin")`)
- Write operations require `settings.write` scope
- Read operations require `settings.read` scope
- Protected by existing `adminIPAccess` middleware
- Uses CSRF protection via `mutationLimiter`

## Database Schema

```sql
CREATE TABLE panel_rate_limit_settings (
  id SERIAL PRIMARY KEY,
  settings JSONB NOT NULL DEFAULT '{}',
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

Single-row table storing all settings as a JSON blob for flexibility and easy schema evolution.
