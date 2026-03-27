package feishu

const (
	maxMessageLen = 4000 // Feishu plain-text message char limit
	maxCardLen    = 4000 // card markdown body char limit

	// File size limits — mirrors Telegram's constants.go
	maxTextFileSize    = 512 * 1024
	maxImageFileSize   = 10 * 1024 * 1024
	maxGenericFileSize = 20 * 1024 * 1024
	maxArchiveEntries  = 20
)
