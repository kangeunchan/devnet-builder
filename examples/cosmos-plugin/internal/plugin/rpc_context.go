package cosmos

import "context"

func (n *CosmosNetwork) requestContext() (context.Context, context.CancelFunc) {
	timeout := n.requestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}
