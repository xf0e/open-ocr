package ocrworker

const (
	PreprocessorIdentity             = "identity"
	PreprocessorStrokeWidthTransform = "stroke-width-transform"
	PreprocessorConvertPdf           = "convert-pdf"
)

type Preprocessor interface {
	preprocess(ocrRequest *OcrRequest) error
}

type IdentityPreprocessor struct{}

func (IdentityPreprocessor) preprocess(_ *OcrRequest) error {
	return nil
}
