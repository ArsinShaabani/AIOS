package sqlstore

// Repositories bundles every repository so the composition root can inject a
// single value (and tests can spin up a temp SQLite file).
type Repositories struct {
	DB            *DB
	Users         *UserRepo
	Sessions      *SessionRepo
	Messages      *MessageRepo
	Memories      *MemoryRepo
	Personalities *PersonalityRepo
	Agents        *AgentRepo
	Devices       *DeviceRepo
	Plugins       *PluginRepo
	Audit         *AuditRepo
	Usage         *UsageRepo
	Settings      *SettingRepo
	Tasks         *TaskRepo
	Tokens        *TokenRepo
	Stats         *StatsRepo
	Documents     *DocumentRepo
	UserProviders *UserProviderRepo
}

// NewRepositories wires the repositories over one database handle.
func NewRepositories(db *DB) *Repositories {
	usage := NewUsageRepo(db)
	return &Repositories{
		DB:            db,
		Users:         NewUserRepo(db),
		Sessions:      NewSessionRepo(db),
		Messages:      NewMessageRepo(db),
		Memories:      NewMemoryRepo(db),
		Personalities: NewPersonalityRepo(db),
		Agents:        NewAgentRepo(db),
		Devices:       NewDeviceRepo(db),
		Plugins:       NewPluginRepo(db),
		Audit:         NewAuditRepo(db),
		Usage:         usage,
		Settings:      NewSettingRepo(db),
		Tasks:         NewTaskRepo(db),
		Tokens:        NewTokenRepo(db),
		Stats:         NewStatsRepo(db, usage),
		Documents:     NewDocumentRepo(db),
		UserProviders: NewUserProviderRepo(db),
	}
}

// Close closes the underlying database.
func (r *Repositories) Close() error {
	if r == nil || r.DB == nil {
		return nil
	}
	return r.DB.Close()
}
