package exitcode

const (
	Success          = 0
	GeneralError     = 1
	InvalidArguments = 2
	AuthError        = 3
	NotFound         = 4
	Conflict         = 5
	ProcessingError  = 6
	NetworkError     = 7
	LockTimeout      = 8
	UploadFailed     = 9
	Interrupted      = 130
)
