// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package ratio_setting

func HasModelTokenRatio(name string) bool {
	_, ok := modelRatioMap.Get(FormatMatchingModelName(name))
	return ok
}
