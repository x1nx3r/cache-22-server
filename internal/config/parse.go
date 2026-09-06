package config

import "strconv"

func sscanfInt(s string, n *int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	*n = v
	return 1, nil
}
