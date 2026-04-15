## 1. Add Helper Methods to AdminHandler

- [x] 1.1 Add `readConfig() (*FullConfig, error)` method to reduce 12 config read/unmarshal blocks
- [x] 1.2 Add `writeConfig(cfg *FullConfig) error` method to handle atomic writes
- [x] 1.3 Add `findProxyByID(id string) (*ExternalProxy, error)` method to reduce 5 duplicate loops
- [x] 1.4 Add keepalive default validation to proxy creation/update logic

## 2. Update Call Sites

- [x] 2.1 Update handleGFWStatus to use readConfig()
- [x] 2.2 Update handleGetGFWConfig to use readConfig()
- [x] 2.3 Update handlePutGFWConfig to use readConfig() and writeConfig()
- [x] 2.4 Update handleRegeneratePAC to use readConfig()
- [x] 2.5 Update handleListProxies to use readConfig()
- [x] 2.6 Update handleCreateProxy to use readConfig() and writeConfig()
- [x] 2.7 Update handleTestProxyByID to use readConfig()
- [x] 2.8 Update handleUpdateProxy to use readConfig(), findProxyByID(), and writeConfig()
- [x] 2.9 Update handleDeleteProxy to use readConfig(), findProxyByID(), and writeConfig()
- [x] 2.10 Update handleGetProxy to use readConfig() and findProxyByID()
- [x] 2.11 Update handleGetConfig to use readConfig()
- [x] 2.12 Update handlePutConfig to use readConfig() and writeConfig()

## 3. Verify

- [x] 3.1 Run tests to verify behavior unchanged
- [x] 3.2 Build succeeds
