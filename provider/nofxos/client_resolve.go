package nofxos

// ResolveClient returns the direct nofxos.ai data client with the default auth key.
func ResolveClient() *Client {
	return NewClient(DefaultBaseURL, DefaultAuthKey)
}
