// API entry point - re-exports from modules for gradual migration
export * from './auth';
export * from './servers';
export * from './mounts';
export * from './files';
export * from './types';
export * from './http';
export * from './rateLimits';
export * from './platform';

// Legacy re-exports from api.ts for backward compatibility
// Admin/management functions that are only defined in the legacy api.ts
export {
  fetchPublicPanelSettings, fetchSetupStatus, runSetup,
  verifyBearerToken,
  fetchUsers, fetchUser,
  fetchNodes, fetchNode, createNode, updateNode, deleteNode, rotateNodeToken,
  fetchNodeConfiguration, fetchNodeAllocations, fetchNodeServers,
  fetchNodeHealth, fetchNodeLifecycle, fetchNodeCapacity, fetchNodeSystemInformation,
  generateNodeConfigToken,
  fetchAllocations, fetchAllocationNodes, createAllocation, updateAllocation, deleteAllocation, deleteAllocations,
  setAllocationAlias, setAdminAllocationAlias, deleteAllocationsBulk, deleteNodeAllocation,
  fetchDatabaseHost, fetchDatabaseHosts, createDatabaseHost, updateDatabaseHost, deleteDatabaseHost, testDatabaseHostConnection,
  deleteServerDatabase,
  fetchMigration, fetchMigrations, createMigration, cancelMigration, prepareMigration, executeMigration,
  previewEvacuation, createEvacuationPlan, fetchEvacuationPlan, executeEvacuationPlan, cancelEvacuationPlan,
  fetchRecoveryPlans, fetchRecoveryPlan, createRecoveryPlan, executeRecoveryPlan, startRecoveryPlan, cancelRecoveryPlan,
  fetchAdminScopes, fetchApiKeys, createApiKey, deleteApiKey,
  fetchSSHKeys, createSSHKey, deleteSSHKey,
  setupTwoFactor, enableTwoFactor, disableTwoFactor,
  fetchActivityLogs, fetchAdminActivity, exportAdminActivity, fetchAdminAudit, fetchAuditEvents,
  type AdminActivityFilter, type AdminActivityPage,
  fetchPermissions, searchUsers,
  fetchPlugins, importPluginFromURL, deletePlugin,
  fetchMyOAuthClients, createMyOAuthClient, deleteMyOAuthClient,
  fetchAdminOAuthClients, createAdminOAuthClient, deleteAdminOAuthClient,
  fetchWebhookDeliveries,
  fetchHealthStatus, fetchReservations,
  fetchRegions, createRegion, updateRegion, deleteRegion,
  fetchLocations, fetchLocation, createLocation, updateLocation, deleteLocation,
  fetchNests, fetchNest, createNest, updateNest, deleteNest,
  fetchEggs, fetchEgg, createEgg, updateEgg, deleteEgg,
  fetchRoles, createRole, deleteRole,
  fetchUserRoles, assignUserRoles, removeUserRoles,
  fetchTemplates,
  fetchPanelSettings, savePanelSettings,
  fetchMailSettings, saveMailSettings, testMailSettings,
  fetchAdvancedSettings, saveAdvancedSettings,
  fetchServerStats, fetchServerLogs,
  fetchServerConfiguration,
  downloadFileToServer, runServerOperations,
  connectServerWebSocket, fetchWSTicket, serverWebSocketURL,
  getBeaconPanelURL,
  rotateServerDatabasePasswordByBody, deleteServerDatabaseWithSuffix,
  createUser, deleteUser, updateUser,
  cancelTransfer, fetchServerTransferStatus,
  getToken, setStoredToken,
} from './legacy';
