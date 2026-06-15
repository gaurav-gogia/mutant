package releaseassets

func normalizeArch(goarch string) string {
	if goarch == "x86" {
		return "386"
	}

	return goarch
}
