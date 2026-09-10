package broker

import "errors"

var (
	ErrConnectorUnknown                 = errors.New("credential connector not found")
	ErrProfileRequired                  = errors.New("credential profile_id or connector_id is required")
	ErrProfileConnectorMismatch         = errors.New("credential profile connector does not match request")
	ErrProfileNotActive                 = errors.New("credential profile is not active")
	ErrDeploymentProfileBindingConflict = errors.New("shared credential was bound by another workspace; refresh and retry")
	ErrProfileManageForbidden           = errors.New("credential profile can only be managed by an explicit manager")
	ErrLastProfileManager               = errors.New("credential profile must keep at least one manager")
	ErrUnsafeParams                     = errors.New("credential crawl params contain sensitive fields")
	ErrWorkerNotConfigured              = errors.New("credential broker worker is not configured")
	ErrWorkerRequestInvalid             = errors.New("credential broker worker rejected request")
	ErrWorkerBusy                       = errors.New("credential broker worker is busy")
	ErrWorkerTimeout                    = errors.New("credential broker worker request timed out")
	ErrWorkerUnavailable                = errors.New("credential broker worker is unavailable")
)
