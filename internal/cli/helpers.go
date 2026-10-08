package cli

import (
	"errors"

	"github.com/azazo1/cnki-cli/internal/cnki"
)

// isVerifyErr 判断错误是否为知网要求重新验证.
func isVerifyErr(err error) bool {
	return errors.Is(err, cnki.ErrVerifyRequired)
}
