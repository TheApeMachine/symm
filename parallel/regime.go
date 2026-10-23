package parallel

import nmstore "github.com/theapemachine/symm/nomagique/store"

/*
regimeLabel picks the brightest lit regime region (R06–R09) on a symbol chord.
*/
func regimeLabel(regions []*nmstore.Region) string {
	var bestKey uint8 = 255
	best := 0.0

	for _, region := range regions {
		if region == nil || region.Key < 6 || region.Key > 9 {
			continue
		}

		if region.Brightness > best {
			best = region.Brightness
			bestKey = region.Key
		}
	}

	switch bestKey {
	case 6:
		return "coiled"
	case 7:
		return "explosive"
	case 8:
		return "turbulent"
	case 9:
		return "dead"
	default:
		return "unknown"
	}
}
