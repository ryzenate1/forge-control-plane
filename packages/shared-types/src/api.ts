// Common API types
export type ApiUser = {
  id: string;
  externalId?: string;
  email: string;
  username?: string;
  nameFirst?: string;
  nameLast?: string;
  role: string;
  rootAdmin?: boolean;
  language?: string;
  cpuLimit?: number;
  memoryMbLimit?: number;
  diskMbLimit?: number;
  backupLimit?: number;
  databaseLimit?: number;
  allocationLimit?: number;
  subuserLimit?: number;
  scheduleLimit?: number;
  serverLimit?: number;
  createdAt: string;
  updatedAt: string;
  sessionVersion?: number;
  disabled?: boolean;
  useTotp?: boolean;
  totpSecret?: string;
};

export type ApiServer = {
  id: string;
  externalId?: string;
  uuid?: string;
  uuidShort?: string;
  name: string;
  description?: string;
  owner?: string;
  ownerId?: string;
  ownerEmail?: string;
  template?: string;
  sftpHost?: string;
  sftpPort?: number;
  permissions?: string[];
  status: string;
  desiredState?: string;
  actualState?: string;
  nodeId?: string;
  node?: string;
  allocationId?: string;
  primaryAllocationId?: string;
  allocation?: string;
  cpuLimit?: number;
  cpuShares?: number;
  memoryMb?: number;
  diskMb?: number;
  swapMb?: number;
  threads?: string;
  oomDisabled?: boolean;
  databaseLimit?: number;
  backupLimit?: number;
  allocationLimit?: number;
  ioWeight?: number;
  suspended?: boolean;
  transferring?: boolean;
  installing?: boolean;
  installationState?: string;
  transferTargetNodeId?: string;
  transferState?: string;
  transferError?: string;
  image?: string;
  createdAt?: string;
  memory?: string | null;
  cpu?: string | null;
  uptime?: string | null;
  featureLimits?: {
    databases: number;
    allocations: number;
    backups: number;
  };
  dockerImage?: string;
  startupCommand?: string;
  environment?: Record<string, string>;
  relationship?: 'owner' | 'subuser' | 'admin';
  configSyncPending?: boolean;
  configSyncError?: string;
  installedAt?: string;
  skipScripts?: boolean;
  dockerLabels?: Record<string, string>;
};

export type ApiNodeActualState = "online" | "offline" | "degraded" | "unknown";

export type ApiNode = {
  id: string;
  uuid?: string;
  name: string;
  region: string;
  regionId?: string;
  locationId?: string;
  status: string;
  desiredState?: string;
  actualState?: ApiNodeActualState;
  heartbeatState?: string;
  heartbeatRecoveryCount?: number;
  maintenanceMode?: boolean;
  draining?: boolean;
  behindProxy?: boolean;
  baseUrl?: string;
  fqdn?: string;
  scheme?: string;
  description?: string;
  public?: boolean;
  isPublic?: boolean;
  tokenId?: string;
  daemonBase?: string;
  daemonListen?: number;
  daemonSftp?: number;
  lastSeenAt?: string;
  memoryMb?: number;
  diskMb?: number;
  uploadSizeMb?: number;
  memoryOverallocate?: number;
  diskOverallocate?: number;
  cpuCores?: number;
  displayName?: string;
  publicHostname?: string;
  listenPortMin?: number;
  listenPortMax?: number;
  allowedIps?: string[];
  networkInterface?: string;
  daemonSslCert?: string;
  daemonSslKey?: string;
  autoConnect?: boolean;
  connectionRetries?: number;
  heartbeatInterval?: number;
  reservedMemoryMb?: number;
  reservedDiskMb?: number;
  defaultAllocationIp?: string;
  allocationPortMin?: number;
  allocationPortMax?: number;
  autoAllocate?: boolean;
  backupDirectory?: string;
  transferDirectory?: string;
  mountPoints?: Record<string, unknown>[];
  tokenRotationPolicy?: string;
  firewallRules?: Record<string, unknown>[];
  tlsSetting?: string;
  enableHealthChecks?: boolean;
  enableMetrics?: boolean;
  prometheusEndpoint?: string;
  alertThresholdCpu?: number;
  alertThresholdMemory?: number;
  alertThresholdDisk?: number;
  maintenanceMessage?: string;
  drainBeforeMaintenance?: boolean;
  labels?: { key: string; value: string }[];
  clusterGroupId?: string;
  daemonSftpAlias?: string;
  daemonConnect?: number;
  cpuOverallocate?: number;
  tags?: string[];
  runtimeStatus?: string;
  runtimeProvider?: string;
  version?: string;
  os?: string;
  architecture?: string;
  cpuThreads?: number;
  dockerStatus?: string;
  nodeMemoryMb?: number;
  nodeDiskMb?: number;
  heartbeatError?: string;
};

export type ApiNodeHealth = {
  cpu: string;
  memory: string;
  disk: string;
  network: string;
  runtime: string;
};

export type ApiNodeHealthScore = {
  cpu: number;
  memory: number;
  disk: number;
  heartbeat: number;
  status: number;
  total: number;
};

export type ApiNodeCapacity = {
  nodeId: string;
  regionId?: string;
  allocated_cpu: number;
  available_cpu: number;
  allocated_memory: number;
  available_memory: number;
  allocated_disk: number;
  available_disk: number;
  server_count: number;
  updated_at: string;
};

export type ApiNodeLifecycle = {
  node: ApiNode;
  health: ApiNodeHealth;
  healthScore: ApiNodeHealthScore;
  capacity: ApiNodeCapacity;
  draining: boolean;
  maintenance: boolean;
  placementEligible: boolean;
  placementBlockedReason?: string;
};

export type ApiAllocationNode = {
  id: string;
  name: string;
};

export type ApiAllocation = {
  id: string;
  node: string;
  nodeId?: string;
  ip: string;
  port: number;
  containerPort?: number;
  protocol?: "tcp" | "udp";
  alias?: string;
  notes?: string;
  server?: string;
  serverId?: string;
  isPrimary?: boolean;
  primary?: boolean;
};

export type ApiDatabase = {
  id: string;
  serverId: string;
  name: string;
  username: string;
  remote?: string;
  password?: string;
  maxConnections?: number;
  hostId?: string;
  host?: string;
  port?: number;
  database?: string;
  engine?: string;
  provisioningState?: "pending" | "ready" | "failed" | string;
  provisioningError?: string;
  createdAt: string;
  updatedAt: string;
};

export type ApiDatabaseOrphanRemediation = {
  id: string;
  serverDatabaseId: string;
  serverId: string;
  databaseHostId: string;
  engine: string;
  host: string;
  port: number;
  database: string;
  username: string;
  remote: string;
  reason: string;
  status: "pending" | "resolved";
  createdAt: string;
  resolvedAt?: string;
};

export type ApiServerOrphanRemediation = {
  id: string;
  serverId: string;
  nodeUrl: string;
  daemonError: string;
  status: "pending" | "resolved";
  createdAt: string;
  resolvedAt?: string;
};

export type ApiOrphanRemediations = {
  serverRemediations: ApiServerOrphanRemediation[];
  databaseRemediations: ApiDatabaseOrphanRemediation[];
};

export type ApiBackup = {
  uuid: string;
  serverId: string;
  name: string;
  id?: string;
  successful?: boolean;
  locked?: boolean;
  isLocked?: boolean;
  status?: string;
  uploadId?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt?: string;
  size?: number;
  checksum?: string;
  statusMessage?: string;
  statusCallback?: string;
  retryCount?: number;
  lastRetryAt?: string;
  ignoredFiles?: string[];
};

export type BackupCreateInput = {
  name?: string;
  ignored?: string[];
  is_locked?: boolean;
};

export type ServerCreateInput = {
  name: string;
  description?: string;
  nodeId?: string;
  regionId?: string;
  region?: string;
  requiredNode?: string;
  preferredNode?: string;
  ownerId: string;
  templateId?: string;
  allocationId?: string;
  additionalAllocationIds?: string[];
  memoryMb?: number;
  cpuShares?: number;
  cpu?: number;
  diskMb?: number;
  databaseLimit?: number;
  backupLimit?: number;
  allocationLimit?: number;
  ioWeight?: number;
  swapMb?: number;
  threads?: string;
  oomDisabled?: boolean;
  dockerImage?: string;
  startupCommand?: string;
  startupVariables?: Record<string, string>;
};

export type ServerUpdateInput = {
  name?: string;
  description?: string;
  ownerId?: string;
  memoryMb?: number;
  cpuShares?: number;
  cpuLimit?: number;
  diskMb?: number;
  databaseLimit?: number;
  backupLimit?: number;
  allocationLimit?: number;
  ioWeight?: number;
  swapMb?: number;
  threads?: string;
  oomDisabled?: boolean;
  dockerImage?: string;
  startupCommand?: string;
  primaryAllocationId?: string;
  allocationId?: string;
};

export type DatabaseCreateInput = {
  name: string;
  remote?: string;
  hostId?: string;
  username?: string;
  password?: string;
};

export type ScheduleCreateInput = {
  name: string;
  cron?: {
    minute: string;
    hour: string;
    dayOfMonth: string;
    month: string;
    dayOfWeek: string;
  };
  cronMinute?: string;
  cronHour?: string;
  cronDayOfMonth?: string;
  cronMonth?: string;
  cronDayOfWeek?: string;
  isActive?: boolean;
  isProcessing?: boolean;
  onlyWhenOnline?: boolean;
  timezone?: string;
  enabled?: boolean;
};

export type ScheduleUpdateInput = {
  name?: string;
  cron?: {
    minute: string;
    hour: string;
    dayOfMonth: string;
    month: string;
    dayOfWeek: string;
  };
  cronMinute?: string;
  cronHour?: string;
  cronDayOfMonth?: string;
  cronMonth?: string;
  cronDayOfWeek?: string;
  isActive?: boolean;
  isProcessing?: boolean;
  onlyWhenOnline?: boolean;
  timezone?: string;
  enabled?: boolean;
};

export type ScheduleTaskCreateInput = {
  action: string;
  payload?: any;
  continueOnFailure?: boolean;
  timeOffset?: number;
  timeOffsetSeconds?: number;
  sequenceId?: string;
  sequence?: number;
  value?: string;
};

export type ScheduleTaskUpdateInput = {
  action?: string;
  payload?: any;
  continueOnFailure?: boolean;
  timeOffset?: number;
  timeOffsetSeconds?: number;
  sequenceId?: string;
  sequence?: number;
  value?: string;
};

export type ApiSchedule = {
  id: string;
  serverId: string;
  name: string;
  cronMinute: string;
  cronHour: string;
  cronDayOfMonth: string;
  cronMonth: string;
  cronDayOfWeek: string;
  onlyWhenOnline: boolean;
  enabled: boolean;
  timezone?: string;
  createdAt: string;
  updatedAt: string;
  lastRunAt?: string;
  nextRunAt?: string;
  tasks?: ApiScheduleTask[];
};

export type ApiScheduleTask = {
  id: string;
  scheduleId: string;
  action: string;
  payload: Record<string, unknown>;
  continueOnFailure: boolean;
  sequenceOrder: number;
  sequence?: number;
  timeOffset?: number;
  timeOffsetSeconds?: number;
};

export type ApiPublicPanelSettings = {
  companyName: string;
  shortName: string;
  productName: string;
  browserTitle: string;
  footerText: string;
  logoUrl: string;
  faviconUrl: string;
  loginBackgroundUrl: string;
  recaptchaSiteKey?: string;
  recaptchaEnabled?: boolean;
  themePreset: string;
  defaultLocale: string;
};

export type ApiSetupStatus = {
  required: boolean;
  hasAdmin: boolean;
  appVersion: string;
};

export type ApiSetupRequest = {
  email: string;
  password: string;
  name?: string;
};

export type LoginResponse = {
  complete: boolean;
  token?: string;
  user?: ApiUser;
  confirmationToken?: string;
};

export type ApiPanelSettings = {
  companyName: string;
  shortName?: string;
  productName?: string;
  browserTitle?: string;
  footerText?: string;
  logoUrl?: string;
  faviconUrl?: string;
  loginBackgroundUrl?: string;
  themePreset?: string;
  defaultLocale: string;
  require2FA?: "none" | "admin" | "all";
  requireEmailVerification?: boolean;
  passwordComplexity?: string;
  passwordExpirationDays?: number;
  sessionDurationMinutes?: number;
  loginRateLimitEnabled?: boolean;
  loginAttemptThreshold?: number;
  accountLockoutMinutes?: number;
  geoRestrictions?: string;
  apiTokenTtlDays?: number;
  apiRotationDays?: number;
  allowedOrigins?: string;
  trustedNetworks?: string;
  defaultTimezone?: string;
  dateFormat?: string;
  numberFormat?: string;
  currencyFormat?: string;
  defaultDashboard?: string;
  landingPage?: string;
  sidebarLayout?: string;
  compactMode?: boolean;
  advancedMode?: boolean;
  metricsRetentionDays?: number;
  logsRetentionDays?: number;
  auditRetentionDays?: number;
  metricsSamplingRate?: number;
  monitoringPollIntervalSeconds?: number;
  emailAlertsEnabled?: boolean;
  webhookAlertsEnabled?: boolean;
  discordWebhookUrl?: string;
  slackWebhookUrl?: string;
  telegramBotToken?: string;
  placementStrategy?: string;
  antiAffinityRules?: string;
  resourceReservationsEnabled?: boolean;
  nodePrioritization?: string;
  recoveryStrategy?: string;
  failoverThresholdSeconds?: number;
  heartbeatThresholdSeconds?: number;
  reservationDurationMinutes?: number;
  reservationCleanupMinutes?: number;
  capacityBufferPercent?: number;
  backupProvider?: string;
  backupRetentionDays?: number;
  backupLimit?: number;
  backupAutoCleanup?: boolean;
  backupEncryptionEnabled?: boolean;
  backupKeyRotationDays?: number;
};

export type CreateServerDatabaseInput = {
  database?: string;
  name?: string;
  hostId?: string;
  remote?: string;
  username?: string;
  password?: string;
  maxConnections?: number;
};

export type ApiWSTicket = {
  token: string;
  expiresAt: string;
};

export type ApiNodeConfiguration = {
  id: string;
  nodeId: string;
  config?: string;
  configFormat?: string;
  token?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type ApiDatabaseHostConnectionTestResult = {
  ok: boolean;
  message?: string;
};

export type ApiServerDatabaseDeleteResult = {
  ok: boolean;
  orphanRemediation?: boolean;
};

export type ApiDatabaseHost = {
  id: string;
  name: string;
  host: string;
  port: number;
  username: string;
  engine: string;
  maxDatabases?: number;
  tlsMode?: string;
  tlsServerName?: string;
  nodeName?: string;
  databases?: number;
  nodeId?: string;
};

export type AdminScopes = Record<string, string>;

export type ApiKey = {
  id: string;
  userId?: string;
  description: string;
  tokenPrefix: string;
  scopes: string[];
  allowedIps?: string[];
  token: string;
  lastUsedAt?: string;
  expiresAt?: string;
  createdAt: string;
};

export type ApiSSHKey = {
  id: string;
  userId?: string;
  name: string;
  publicKey: string;
  fingerprint: string;
  createdAt: string;
};

export type ApiOAuthClient = {
  id: string;
  name: string;
  description?: string;
  scopes: string[];
  allowedScopes: string[];
  scope: string;
  clientId?: string;
  clientSecretHash?: string;
  ownerId?: string;
  createdAt: string;
  updatedAt: string;
};

export type ApiOAuthClientCreation = {
  id: string;
  name: string;
  description?: string;
  scopes: string[];
  clientId?: string;
  clientSecret?: string;
  client?: { id: string; clientId: string; name: string; scopes: string[] };
  createdAt: string;
};

export type ApiPlugin = {
  id: string;
  name: string;
  description?: string;
  version?: string;
  author?: string;
  enabled: boolean;
  kind?: string;
  installedAt: string;
};

export type ApiWebhook = {
  id: string;
  name: string;
  description?: string;
  url: string;
  events: string[];
  enabled: boolean;
  secret?: string;
  webhookType?: string;
  discordUsername?: string;
  discordAvatarUrl?: string;
  discordContent?: string;
  createdAt: string;
  updatedAt: string;
};

export type ApiWebhookDelivery = {
  id: string;
  webhookId?: string;
  eventName: string;
  targetUrl?: string;
  webhookType?: string;
  event?: string;
  state: string;
  status?: string;
  statusCode?: number;
  responseStatus?: number;
  responseBody?: string;
  responseBodyExcerpt?: string;
  attempts: number;
  attempt?: number;
  lastError?: string;
  nextAttemptAt?: string;
  deliveredAt?: string;
  createdAt: string;
};

export type ApiMigrationStatus =
  | "pending"
  | "planned"
  | "preparing"
  | "transferring"
  | "restoring"
  | "in_progress"
  | "completed"
  | "failed"
  | "cancelled";

export type ApiMigrationHistory = {
  id: string;
  migrationId: string;
  serverId: string;
  sourceNodeId: string;
  targetNodeId: string;
  status: ApiMigrationStatus;
  fromStatus: string;
  toStatus: string;
  reason: string;
  startedAt?: string;
  completedAt?: string;
  error?: string;
  createdAt: string;
};

export type ApiMigration = {
  id: string;
  serverId: string;
  sourceNodeId: string;
  targetNodeId: string;
  status: ApiMigrationStatus;
  initiatedBy?: string;
  priority?: number;
  transferMethod?: string;
  transferPhase?: string;
  idempotencyKey?: string;
  archiveSize?: number;
  archiveChecksum?: string;
  cleanupPending?: boolean;
  history?: ApiMigrationHistory[];
  progress?: number;
  error?: string;
  failureReason?: string;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateMigrationInput = {
  serverId: string;
  sourceNodeId?: string;
  targetNodeId?: string;
};

export type ApiPanelMailSettings = {
  enabled?: boolean;
  mailFrom?: string;
  mailFromAddress?: string;
  mailFromName?: string;
  host?: string;
  smtpHost?: string;
  port?: number;
  smtpPort?: number;
  encryption?: string;
  smtpEncryption?: string;
  username?: string;
  smtpUsername?: string;
  password?: string;
  smtpPassword?: string;
};

export type ApiPanelAdvancedSettings = {
  reCAPTCHAEnabled?: boolean;
  reCAPTCHASiteKey?: string;
  reCAPTCHASecretKey?: string;
  recaptchaEnabled?: boolean;
  recaptchaWebsiteKey?: string;
  recaptchaSecretKey?: string;
  analyticsCode?: string;
  maintenanceMode?: boolean;
  maintenanceMessage?: string;
  guzzleConnectTimeout?: number;
  guzzleRequestTimeout?: number;
  autoAllocEnabled?: boolean;
  autoAllocStartPort?: number;
  autoAllocEndPort?: number;
};

export type ApiMount = {
  id: string;
  uuid?: string;
  name: string;
  description?: string;
  source: string;
  target: string;
  readOnly: boolean;
  userMountable?: boolean;
  nodeIds?: string[];
  templateIds?: string[];
  serverIds?: string[];
  eggs?: string[];
  nodes?: string[];
  servers?: string[];
};

export type ApiNest = {
  id: string;
  name: string;
  description?: string;
  eggs?: number;
  eggCount?: number;
  createdAt: string;
};

export type ApiEgg = {
  id: string;
  nestId: string;
  name: string;
  description?: string;
  dockerImages?: Record<string, string>;
  dockerImage?: string;
  dockerImagesList?: string[];
  startup: string;
  startupCommand?: string;
  config?: any;
  configFiles?: string;
  environment?: Record<string, string>;
  variables?: ApiStartupVariable[];
  installScript?: string;
  installContainer?: string;
  installEntrypoint?: string;
  image?: string;
  fileDenylist?: string[];
  defaultMemoryMb?: number;
  nestName?: string;
  createdAt: string;
};

export type ApiRole = {
  id: string;
  key: string;
  name: string;
  isAdmin: boolean;
  createdAt: string;
};

export type ApiRegion = {
  id: string;
  uuid: string;
  name: string;
  slug: string;
  description: string;
  enabled: boolean;
  nodeCount: number;
  createdAt: string;
  updatedAt: string;
};

export type ApiLocation = {
  id: string;
  short: string;
  long: string;
  nodeCount: number;
  serverCount: number;
  createdAt: string;
};

export type ApiAdminAuditEvent = {
  id: string;
  userId: string;
  userEmail?: string;
  action: string;
  resource: string;
  resourceId?: string;
  metadata?: Record<string, unknown>;
  ip?: string;
  userAgent?: string;
  actorEmail?: string;
  targetType?: string;
  targetId?: string;
  createdAt: string;
};

export type ApiStats = {
  cpuPercent: number;
  memoryBytes: number;
  memoryLimit: number;
  diskBytes: number;
  diskLimit: number;
  networkRxBytes: number;
  networkTxBytes: number;
  uptime: number;
  uptimeMs?: number;
  state?: string;
};

export type TwoFactorSetup = {
  enabled: boolean;
  secret?: string;
  qrCodeUrl?: string;
  qrCode?: string;
  image_url?: string;
  tokens?: string[];
};

export type CreateNodeInput = {
  name: string;
  region: string;
  regionId?: string;
  locationId?: string;
  description?: string;
  baseUrl?: string;
  fqdn: string;
  scheme?: string;
  behindProxy?: boolean;
  public?: boolean;
  memoryMb?: number;
  diskMb?: number;
  uploadSizeMb?: number;
  daemonBase?: string;
  daemonListen?: number;
  daemonSftp?: number;
  memoryOverallocate?: number;
  diskOverallocate?: number;
  cpuCores?: number;
  displayName?: string;
  publicHostname?: string;
  maintenanceMode?: boolean;
  daemonSftpAlias?: string;
  daemonConnect?: number;
  cpuOverallocate?: number;
  tags?: string[];
};

export type UpdateNodeInput = {
  name?: string;
  description?: string;
  locationId?: string;
  baseUrl?: string;
  fqdn?: string;
  scheme?: string;
  behindProxy?: boolean;
  public?: boolean;
  maintenanceMode?: boolean;
  desiredState?: string;
  draining?: boolean;
  memoryMb?: number;
  diskMb?: number;
  uploadSizeMb?: number;
  daemonBase?: string;
  daemonListen?: number;
  daemonSftp?: number;
  status?: string;
  memoryOverallocate?: number;
  diskOverallocate?: number;
  cpuCores?: number;
  displayName?: string;
  publicHostname?: string;
  daemonSftpAlias?: string;
  daemonConnect?: number;
  cpuOverallocate?: number;
  tags?: string[];
};

export type CreateAllocationInput = {
  nodeId: string;
  ip: string;
  ports: string;
  containerPort?: number;
  protocol?: "tcp" | "udp";
  alias?: string;
  notes?: string;
};

export type UpdateAllocationInput = {
  alias?: string;
  notes?: string;
};

export type CreateDatabaseHostInput = {
  name: string;
  host: string;
  port: number;
  username: string;
  password: string;
  engine?: string;
  tlsMode?: string;
  tlsServerName?: string;
  tlsCa?: string;
  nodeId?: string;
  maxDatabases?: number;
};

export type UpdateDatabaseHostInput = {
  name?: string;
  host?: string;
  port?: number;
  username?: string;
  password?: string;
  engine?: string;
  tlsMode?: string;
  tlsServerName?: string;
  tlsCa?: string;
  nodeId?: string;
  maxDatabases?: number;
};

export type CreateMountInput = {
  name: string;
  source: string;
  target: string;
  readOnly: boolean;
  description?: string;
  userMountable?: boolean;
  nodeIds?: string[];
  templateIds?: string[];
};

export type AssignMountInput = {
  mountId: string;
  readOnly?: boolean;
};

export type ApiMountAssignmentResponse = {
  ok: boolean;
  runtimeSynchronized: boolean;
};

export type RenameFileInput = {
  from: string;
  to: string;
};

export type PatchScheduleTaskInput = {
  action?: string;
  payload?: any;
  continueOnFailure?: boolean;
  timeOffset?: number;
  timeOffsetSeconds?: number;
  sequence?: number;
  value?: string;
};

export type CreateEggInput = {
  name: string;
  nestId: string;
  description?: string;
  dockerImages?: Record<string, string> | string[];
  startup?: string;
  config?: any;
  defaultMemoryMb?: number;
  installScript?: string;
  installContainer?: string;
  installEntrypoint?: string;
  fileDenylist?: string[];
};

export type UpdateEggInput = {
  name?: string;
  description?: string;
  dockerImages?: Record<string, string> | string[];
  startup?: string;
  config?: any;
  defaultMemoryMb?: number;
  installScript?: string;
  installContainer?: string;
  installEntrypoint?: string;
  fileDenylist?: string[];
};

export type ApiActivityLog = {
  id: string;
  userId?: string;
  action?: string;
  resource?: string;
  description?: string;
  event: string;
  timestamp: string;
  resourceId?: string;
  metadata?: Record<string, unknown>;
  ip?: string;
  userAgent?: string;
  actorEmail?: string;
  subjectType?: string;
  subjectId?: string;
  properties?: any;
  level?: string;
  source?: string;
  createdAt?: string;
};

export type ApiAuditEvent = {
  id: string;
  userId?: string;
  actorEmail?: string;
  action: string;
  targetType?: string;
  targetId?: string;
  metadata?: Record<string, unknown>;
  ip?: string;
  createdAt: string;
};

export type ApiFileEntry = {
  name: string;
  path: string;
  directory: boolean;
  size?: number;
  mode?: string;
  mime?: string;
  symlink?: boolean;
  modifiedAt?: string;
  createdAt?: string;
};

export type ApiServerSubuser = {
  id: string;
  userId?: string;
  email: string;
  permissions: string[];
  createdAt?: string;
  updatedAt?: string;
};

export type ApiStartupVariable = {
  id: string;
  name: string;
  description?: string;
  envVariable: string;
  env_variable?: string;
  defaultValue: string;
  default_value?: string;
  serverValue: string;
  server_value?: string;
  rules: string;
  is_editable?: boolean;
};

export type CrashEvent = {
  id: string;
  server_id: string;
  node_id: string;
  exit_code: number;
  oom_killed: boolean;
  clean_exit: boolean;
  auto_restarted: boolean;
  crash_count: number;
  node_state: Record<string, unknown> | null;
  created_at: string;
};

export type ApiHealthCheck = {
  name: string;
  status: "ok" | "warning" | "failed";
  label: string;
  notificationMessage: string;
  critical: boolean;
  latencyMs?: number;
  details?: Record<string, unknown>;
  lastChecked?: string;
  lastSuccess?: string;
  lastFailure?: string;
  consecutiveFailures?: number;
};

export type ApiHealthReport = {
  status: "ok" | "warning" | "failed";
  ok: boolean;
  service: string;
  version?: string;
  uptime?: string;
  checks: ApiHealthCheck[];
  checkedAt: string;
};

export type ApiTemplate = {
  id: string;
  name: string;
  description?: string;
  eggId: string;
  nestId: string;
  dockerImage?: string;
  startupCommand?: string;
  environment?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
};

export type ApiUserSearchResult = {
  users: ApiUser[];
  total: number;
  page: number;
  perPage: number;
};

export type ApiUserSession = {
  id: string;
  uuid?: string;
  userId?: string;
  ip?: string;
  ipAddress: string;
  userAgent?: string;
  lastActivity?: string;
  createdAt?: string;
  current?: boolean;
  isRevoked?: boolean;
  revokedAt?: string;
  revokeReason?: string;
  expiresAt?: string;
};

export type ApiEvacuationItem = {
  id: string;
  planId: string;
  serverId: string;
  sourceNodeId: string;
  targetNodeId?: string;
  eligible: boolean;
  reason: string;
  migrationId?: string;
  status: string;
  error?: string;
};

export type ApiEvacuationPlan = {
  id: string;
  nodeId: string;
  status: "pending" | "running" | "completed" | "cancelled" | "failed";
  items: ApiEvacuationItem[];
  createdAt: string;
  updatedAt: string;
};

export type ApiEvacuationResult = {
  plan: ApiEvacuationPlan;
  items: ApiEvacuationItem[];
  preview: boolean;
};

export type ApiRecoveryItem = {
  id: string;
  planId: string;
  serverId: string;
  sourceNodeId: string;
  targetNodeId?: string;
  reservationId?: string;
  migrationId?: string;
  sourceBackupName?: string;
  sourceBackupChecksum?: string;
  sourceBackupSize?: number;
  status: "pending" | "planned" | "executing" | "completed" | "restored" | "cancelled" | "failed" | "skipped";
  reason?: string;
  createdAt: string;
  updatedAt: string;
};

export type ApiReservation = {
  id: string;
  nodeId: string;
  status: string;
  serverId?: string;
  migrationId?: string;
  reservationType?: string;
  reservedBy?: string;
  cpu?: number;
  memory?: number;
  disk?: number;
  createdAt?: string;
  updatedAt?: string;
};

export type ApiRecoveryPlan = {
  id: string;
  nodeId: string;
  status: "pending" | "planning" | "planned" | "executing" | "completed" | "restored" | "cancelled" | "failed";
  reason: string;
  items: ApiRecoveryItem[];
  createdAt: string;
  updatedAt: string;
};

export type CreateRecoveryPlanInput = {
  nodeId: string;
  reason: string;
};

export type ApiLegacyTransferStatus = {
  state?: string;
  transferring: boolean;
  targetNodeId?: string;
  error?: string;
};

export type SocialProvider = {
  id: string;
  name: string;
  displayName: string;
  enabled: boolean;
  clientId: string;
  issuerUrl?: string;
  hasClientSecret: boolean;
  scopes: string[];
  buttonStyle: string;
  iconClass: string;
  createdAt: string;
  updatedAt: string;
};
