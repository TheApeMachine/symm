package hindsight

import "time"

/*
Lesson selects a symbol's complete captured session for rehearsal. Its marks
are retrospective evaluation coordinates, never decision inputs. Keeping the
whole session includes the lead-in, stagnation, reversal and quiet intervals;
a replay does not begin a fresh wallet precisely at a hindsight price minimum.
*/
type Lesson struct {
	Run     RunID           `json:"run"`
	Symbol  string          `json:"symbol"`
	From    CaptureSequence `json:"from"`
	Through CaptureSequence `json:"through"`
	Marks   []Episode       `json:"marks"`
	Horizon time.Duration   `json:"horizonNs"`
}

/*
Lessons selects confirmed spot price episodes using canonical Hindsight
geometry. Horizon is the mean completed leg duration, a training-data estimate
of the time scale to measure, not an entry or exit timestamp. The caller must
freeze it before evaluating later tape. Quiet intervals remain in every lesson.
*/
func (index *RunIndex) Lessons(policy DiscoveryPolicy) []Lesson {
	lessons := []Lesson{}
	for _, summary := range index.Summaries(policy) {
		lesson := Lesson{Run: index.run, Symbol: summary.Symbol, From: summary.FirstSeq, Through: summary.LastSeq}
		count := 0
		mean := 0.0
		for _, episode := range index.Discover(summary.Symbol, policy).Episodes {
			if !episode.Confirmed || (episode.Kind != EpisodeUpwardExcursion && episode.Kind != EpisodeDownwardExcursion) {
				continue
			}
			elapsed := episode.ToAt.Sub(episode.FromAt)
			if elapsed <= 0 {
				continue
			}
			count++
			mean += (float64(elapsed) - mean) / float64(count)
			lesson.Marks = append(lesson.Marks, episode)
		}
		if count == 0 {
			continue
		}
		lesson.Horizon = time.Duration(mean)
		lessons = append(lessons, lesson)
	}
	return lessons
}

/* RehearsalInput is an immutable numerical decision input, without hindsight labels or old actions. */
type RehearsalInput struct {
	Capture     CaptureIdentity `json:"capture"`
	Symbol      string          `json:"symbol"`
	At          time.Time       `json:"at"`
	GridVersion uint64          `json:"gridVersion"`
	Context     []uint64        `json:"context"`
	Quantities  [][2]string     `json:"quantities"`
	Authority   float64         `json:"authority"`
}
