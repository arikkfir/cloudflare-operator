package internal

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) []string {
	if slice == nil || len(slice) == 0 {
		return slice
	}

	//goland:noinspection GoPreferNilSlice
	result := []string{}
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return result
}
