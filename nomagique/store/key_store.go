package store

/*
KeyStore maintains bounded state stores partitioned per key.
When configured on a primitive or algo, it provides history isolation
per key without duplicating the pipeline graph.
*/
type KeyStore struct {
	Key     func() string
	Windows map[string]*Store
}

/*
NewKeyStore constructs a KeyStore with the given key selector.
*/
func NewKeyStore(key func() string) *KeyStore {
	return &KeyStore{
		Key:     key,
		Windows: make(map[string]*Store),
	}
}

/*
Get returns the Store for the given key, creating one if it does not exist.
*/
func (keyStore *KeyStore) Get(key string) *Store {
	if keyStore.Windows == nil {
		keyStore.Windows = make(map[string]*Store)
	}

	storeInstance, found := keyStore.Windows[key]

	if !found {
		storeInstance = &Store{}
		keyStore.Windows[key] = storeInstance
	}

	return storeInstance
}

/*
Active returns the Store for the current key returned by Key().
*/
func (keyStore *KeyStore) Active() *Store {
	if keyStore.Key == nil {
		return nil
	}

	return keyStore.Get(keyStore.Key())
}
