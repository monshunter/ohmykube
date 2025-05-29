package log

import (
	"fmt"
	"strings"
	"time"
)

// ProgressBar represents a progress bar for long-running operations
type ProgressBar struct {
	title       string
	total       int
	current     int
	width       int
	startTime   time.Time
	lastUpdate  time.Time
	completed   bool
	showTime    bool
}

// NewProgressBar creates a new progress bar
func NewProgressBar(title string, total int) *ProgressBar {
	return &ProgressBar{
		title:     title,
		total:     total,
		width:     20,
		startTime: time.Now(),
		showTime:  true,
	}
}

// Update updates the progress bar
func (pb *ProgressBar) Update(current int) {
	if quiet {
		return // Don't show progress in quiet mode
	}
	
	pb.current = current
	pb.lastUpdate = time.Now()
	pb.render()
}

// Increment increments the progress by 1
func (pb *ProgressBar) Increment() {
	pb.Update(pb.current + 1)
}

// Complete marks the progress as completed
func (pb *ProgressBar) Complete() {
	pb.current = pb.total
	pb.completed = true
	pb.render()
	fmt.Println() // New line after completion
}

// render renders the progress bar
func (pb *ProgressBar) render() {
	if quiet {
		return
	}

	percentage := float64(pb.current) / float64(pb.total) * 100
	filled := int(float64(pb.width) * float64(pb.current) / float64(pb.total))
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", pb.width-filled)
	
	timeStr := ""
	if pb.showTime && pb.completed {
		elapsed := time.Since(pb.startTime)
		timeStr = fmt.Sprintf(" (%s)", formatDuration(elapsed))
	}
	
	status := "100%"
	if !pb.completed {
		status = fmt.Sprintf("%.0f%%", percentage)
	}
	
	fmt.Printf("\r%s [%s] %s%s", pb.title, bar, status, timeStr)
	
	if pb.completed {
		fmt.Print(" ✅")
	}
}

// formatDuration formats duration to human readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// Step represents a single step in a multi-step process
type Step struct {
	Name        string
	Description string
	Progress    *ProgressBar
	StartTime   time.Time
	EndTime     time.Time
	Completed   bool
	Error       error
}

// MultiStepProgress manages multiple steps with progress tracking
type MultiStepProgress struct {
	steps       []*Step
	currentStep int
	title       string
}

// NewMultiStepProgress creates a new multi-step progress tracker
func NewMultiStepProgress(title string) *MultiStepProgress {
	return &MultiStepProgress{
		title: title,
		steps: make([]*Step, 0),
	}
}

// AddStep adds a new step to the progress tracker
func (msp *MultiStepProgress) AddStep(name, description string) {
	step := &Step{
		Name:        name,
		Description: description,
	}
	msp.steps = append(msp.steps, step)
}

// StartStep starts the specified step
func (msp *MultiStepProgress) StartStep(stepIndex int) {
	if quiet {
		return
	}
	
	if stepIndex >= len(msp.steps) {
		return
	}
	
	msp.currentStep = stepIndex
	step := msp.steps[stepIndex]
	step.StartTime = time.Now()
	
	// Show step start
	icon := getStepIcon(stepIndex)
	fmt.Printf("\n%s %s", icon, step.Description)
}

// CompleteStep completes the current step
func (msp *MultiStepProgress) CompleteStep(stepIndex int) {
	if stepIndex >= len(msp.steps) {
		return
	}
	
	step := msp.steps[stepIndex]
	step.EndTime = time.Now()
	step.Completed = true
	
	if !quiet {
		elapsed := step.EndTime.Sub(step.StartTime)
		fmt.Printf(" ✅ (%s)\n", formatDuration(elapsed))
	}
}

// FailStep marks a step as failed
func (msp *MultiStepProgress) FailStep(stepIndex int, err error) {
	if stepIndex >= len(msp.steps) {
		return
	}
	
	step := msp.steps[stepIndex]
	step.EndTime = time.Now()
	step.Error = err
	
	if !quiet {
		fmt.Printf(" ❌ Failed: %v\n", err)
	}
}

// Complete completes the entire multi-step process
func (msp *MultiStepProgress) Complete() {
	if quiet {
		return
	}
	
	totalTime := time.Duration(0)
	for _, step := range msp.steps {
		if step.Completed {
			totalTime += step.EndTime.Sub(step.StartTime)
		}
	}
	
	fmt.Printf("\n✅ %s completed in %s!\n", msp.title, formatDuration(totalTime))
}

// getStepIcon returns an icon for the step based on its index
func getStepIcon(stepIndex int) string {
	icons := []string{"📦", "🔧", "🔗", "🌐", "💾", "⚙️", "🚀"}
	if stepIndex < len(icons) {
		return icons[stepIndex]
	}
	return "▶️"
}

// Simple progress functions for backward compatibility

// ProgressInfo shows progress information (only in non-quiet mode)
func ProgressInfo(args ...any) {
	if !quiet {
		Info(args...)
	}
}

// ProgressInfof shows formatted progress information (only in non-quiet mode)
func ProgressInfof(format string, args ...any) {
	if !quiet {
		Infof(format, args...)
	}
}

// QuietInfo shows info only in verbose mode, hidden in quiet mode
func QuietInfo(args ...any) {
	if verbose {
		Info(args...)
	}
}

// QuietInfof shows formatted info only in verbose mode, hidden in quiet mode
func QuietInfof(format string, args ...any) {
	if verbose {
		Infof(format, args...)
	}
}
