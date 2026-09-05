package d2parser_test

// Frozen AST, error, position, and reader-call digests from 5f01776201890014cbe1131e87e89db891fa5b24.
var parserBufferLegacyDigests = map[string]string{
	"empty":                "da6530fb9a3c1527e0a09153d468a561fe95573bccb37b482f669b76de3063d8",
	"ordinary":             "56ae1a8b2ed4da386b6320d2e73557fb0ae84f8b201c50af1b13bd58e6ad8366",
	"raw_escape":           "358b5068d0f97244b2b1318c97788daf366702af12f2c1c22798a35998bdf378",
	"substitutions":        "49d113aad13ba5bd74974e38dd025c1100004c1cbdc7ab108370bac9b31a54b9",
	"quoted":               "de207464cc7e1d308375f7b6f0f18e0788b45c86013208971e4c31cf50590a5f",
	"escaped_newlines":     "179950a3c271641e2a1d1f779908d467389ded6c81689cacd74f673c3718be86",
	"patterns":             "7edb6c054e6380cff7346116d02f8b72eb5ca0f9808390f573c098c726510984",
	"edge_groups":          "281bfac475e68d6fa3cd54db701271846fc1b4e6f9d3cd2ddcfc64d2dc35a411",
	"crlf_unicode":         "b66c0ac6d7fe7d75e6e5cd81f42f9c8dd21fb465fe7b50bdf648b11a65142933",
	"invalid_utf8":         "153b53180a4f7d0128b978e095e64000415ec6288991971f371219153448c17a",
	"utf16_bom":            "3a4329f882a544072f1600afbe4313d0e68239efa386fd3b16d8bd134c2bc948",
	"short_prefixes":       "707b8c80677001a33270705272fe403b2dee4b7ea0a2b185e51f30f8f676fa50",
	"eof_escape":           "575a6b9f2dcea38a345f08d2b4f6dc6aec5c809fdefeb9b97ccad4f0e23f60e6",
	"eof_dash":             "8f11d0e0db5b47c4bc76aab01c8ebebbeffe102b3602bccaa60a190b7142d2b3",
	"eof_quote":            "787a6980d65354d7251370cb4e23857dd43f510a64e9ab6601ca36f49cc03a54",
	"comments_blocks":      "1e6b1ef546b96b5610539a9bb1798ad0eafd279cf32b83c9ffd700abd2aee174",
	"long_lookahead":       "45a380987406395ccf64b5e23a656b5e490423d928f19227bde4a765b1b54b24",
	"whitespace_rewind":    "f6b84b47209abeb251fb12de7d762158657991ccdad5cc06e62c26a40d3f247b",
	"delimiters":           "092f1a3dd7fd1f1796dce9cf4da253da6f8c57952ba1f0b94a86173981ebecd2",
	"reader_error_initial": "569e1dd11f2b25f688662fc6062445e6bf052f0068fec8781a69f36d07984b11",
	"reader_error_prefix":  "9f91836eb59cae16969478390311c9ab8b07dc6a6a9bfcf34dc10b774ca4c0a7",
	"reader_error_escape":  "c717d9044912b6ece4a4230c9fdf7f56b564d0d5830ab87525df1d3fbcbbf111",
}
