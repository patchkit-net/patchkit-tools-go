package native

// DeltaAlgorithm specifies which delta algorithm to use.
type DeltaAlgorithm string

const (
	AlgorithmLibrsync   DeltaAlgorithm = "librsync"
	AlgorithmTurbopatch DeltaAlgorithm = "turbopatch"
)

// Rsync provides file signature and delta operations.
type Rsync interface {
	// Signature generates a librsync signature for the given file.
	Signature(filePath, sigOutputPath string, blockLen int) error

	// Delta generates a delta from a signature and new file.
	Delta(sigPath, newFilePath, deltaOutputPath string) error
}

// TurboPatch provides turbopatch delta operations.
type TurboPatch interface {
	// Delta2 generates a turbopatch delta from old sig, new sig, and new file.
	Delta2(oldSigPath, newSigPath, newFilePath, deltaOutputPath string, blockSize int64) error
}

// ReadSignatureBlockLen reads the block length from a librsync signature file header.
// Format: 4 bytes magic + 4 bytes block_len (big-endian)
func ReadSignatureBlockLen(sigPath string) (int, error) {
	return readSigBlockLen(sigPath)
}
