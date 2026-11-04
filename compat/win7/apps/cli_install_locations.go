package apps

import (
	"csgclaw/internal/connectorcli"
	"maps"
)

// Saved connectors pin package bytes, but must still receive corrected download
// locations for those same bytes when a failed installation is retried.
func (s *Service) currentCLIInstallSpec(spec connectorcli.InstallSpec) connectorcli.InstallSpec {
	for _, pkg := range s.packages {
		if pkg.CLI == nil || pkg.CLI.Install.Version != spec.Version || pkg.CLI.Install.Executable != spec.Executable {
			continue
		}
		spec.Releases = maps.Clone(spec.Releases)
		for platform, archive := range spec.Releases {
			if current, ok := pkg.CLI.Install.Releases[platform]; ok && current.SHA256 == archive.SHA256 {
				archive.URL = current.URL
				spec.Releases[platform] = archive
			}
		}
		break
	}
	return spec
}
