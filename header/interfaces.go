package header

// Sequenceable is an interface that any header type must implement
// to work with the processor. It represents a header that can be ordered in a sequence.
type Sequenceable[T any, ID comparable] interface {
	// GetSequenceID returns the sequence identifier of the header
	GetSequenceID() ID

	// IsNext checks if this header comes immediately after the previous header
	IsNext(prev T) bool
}

// HeaderHandler defines how headers should be processed
type HeaderHandler[T any, ID comparable] interface {
	// Handle processes a header and returns an error if processing fails
	Handle(header T) error
}

// HeaderStore provides storage capabilities for headers
type HeaderStore[T any, ID comparable] interface {
	// Store saves a header to the store
	Store(header T) error

	// GetLatest returns the latest header from the store
	GetLatest() (T, error)
}

// HeaderFetcher provides functionality to fetch specific headers
type HeaderFetcher[T any, ID comparable] interface {
	// FetchByID fetches a header with a specific sequence ID
	FetchByID(id ID) (T, error)
}

// HeaderSubscriber provides functionality to subscribe to a stream of headers
type HeaderSubscriber[T any, ID comparable] interface {
	// Subscribe returns a channel that receives headers
	Subscribe() (<-chan T, error)

	// Unsubscribe closes the subscription
	Unsubscribe() error
}

// SequenceComparator provides methods to compare sequence IDs
type SequenceComparator[ID comparable] interface {
	// IsGreaterThan checks if a is greater than b
	IsGreaterThan(a, b ID) bool

	// IsEqual checks if a equals b
	IsEqual(a, b ID) bool

	// Next returns the next sequence ID after the given one
	Next(id ID) ID

	// Distance returns the distance between two sequence IDs
	Distance(from, to ID) uint64
}
