package limits

func exceeds(n int) error {
	if n > max {
		return errTooBig
	}
	return nil
}
