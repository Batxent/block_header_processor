package examples

// HeaderSubscriberAdapter adapts HeaderService to the HeaderSubscriber interface
type HeaderSubscriberAdapter struct {
	service *HeaderService
	ch      <-chan BlockHeader
	cancel  func()
}

// NewHeaderSubscriber creates a new mock header subscriber
func NewHeaderSubscriber(service *HeaderService) *HeaderSubscriberAdapter {
	return &HeaderSubscriberAdapter{
		service: service,
	}
}

// Subscribe implements the HeaderSubscriber interface
func (a *HeaderSubscriberAdapter) Subscribe() (<-chan BlockHeader, error) {
	ch, cancel, err := a.service.Subscribe(100)
	if err != nil {
		return nil, err
	}

	a.ch = ch
	a.cancel = cancel
	return ch, nil
}

// Unsubscribe implements the HeaderSubscriber interface
func (a *HeaderSubscriberAdapter) Unsubscribe() error {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	return nil
}

// HeaderFetcherAdapter adapts HeaderService to the HeaderFetcher interface
type HeaderFetcherAdapter struct {
	service *HeaderService
}

// NewHeaderFetcher creates a new mock header fetcher
func NewHeaderFetcher(service *HeaderService) *HeaderFetcherAdapter {
	return &HeaderFetcherAdapter{
		service: service,
	}
}

// FetchByID implements the HeaderFetcher interface
func (a *HeaderFetcherAdapter) FetchByID(id uint64) (BlockHeader, error) {
	return a.service.GetHeader(id)
}
