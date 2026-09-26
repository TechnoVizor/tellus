// Package i18n looks up UI strings. Only English exists for now, but every
// user-facing string goes through T so adding a language later is cheap.
package i18n

// T returns the English text for key, or the key itself when it is unknown.
func T(key string) string {
	if s, ok := en[key]; ok {
		return s
	}
	return key
}
