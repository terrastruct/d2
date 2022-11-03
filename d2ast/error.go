package d2ast

// TODO: Right now this is here to be available in both the Parser and Compiler but
// eventually we should make this a real part of the AST so that autofmt works on
// files with parse errors and semantically it makes more sense.
// Compile would continue to maintain a separate set of errors and then we'd do a
// merge & sort to get the final list of errors for user display.
type Error struct {
	Range   Range  `json:"range"`
	Message string `json:"errmsg"`
}

func (e Error) Error() string {
	return e.Message
}
