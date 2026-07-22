package broker

import "errors"

var (
	ErrConnectorUnknown         = errors.New("credential connector not found")
	ErrProfileRequired          = errors.New("credential profile_id or connector_id is required")
	ErrProfileConnectorMismatch = errors.New("credential profile connector does not match request")
	ErrProfileNotActive         = errors.New("credential profile is not active")
	ErrUnsafeParams             = errors.New("credential crawl params contain sensitive fields")
	ErrWorkerNotConfigured      = errors.New("credential broker worker is not configured")
)
