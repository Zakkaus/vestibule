package rules

import "testing"

// kernelRule is the interval set the join challenge grades against.
var kernelRule = VersionRange{Intervals: []VersionInterval{
	{Minimum: Version{Major: 0, Minor: 1}, Maximum: Version{Major: 0, Minor: 99}},
	{Minimum: Version{Major: 1}, Maximum: Version{Major: 1, Minor: 3}},
	{Minimum: Version{Major: 2}, Maximum: Version{Major: 2, Minor: 6}},
	{Minimum: Version{Major: 3}, Maximum: Version{Major: 30, Minor: 99}},
}}

// A plain answer carries one release. A reply carrying a second one is a paste of something else,
// and grading only the first would approve the account with whatever was written after it.
func TestOnlyAReplyWithASingleReleaseIsGradedAsAPlainAnswer(t *testing.T) {
	if !kernelRule.Matches("linux 6.12.3 #1 smp preempt_dynamic") {
		t.Fatal("the same shape carrying one release was refused, so the refusals below prove nothing")
	}
	for _, text := range []string{
		"linux 6.12.3 #1 buy-vpn.example 4.4.4",
		"linux 6.12.3 #1 t.me/channel 1.2.3",
		"6.12.3 or maybe 5.10.1",
	} {
		if kernelRule.Matches(text) {
			t.Errorf("Matches(%q) = true: a reply carrying a second version was approved with an advertisement as its verification answer", text)
		}
	}
}

// The allow-list is what lets a real answer be written as a sentence, and what lets a uname line
// name an architecture. Drop or mistype one entry and the users it covers are graded wrong, charged
// one of three tries, and banned on the third.
func TestEveryAllowedContextWordMayStandBesideAKernelRelease(t *testing.T) {
	words := []string{
		"#1", "-a", "-r", "-sr",
		"linux", "uname", "gnu/linux",
		"smp", "preempt", "preempt_dynamic",
		"x86_64", "amd64", "aarch64", "arm64", "i686",
		"armv7l", "armv8l", "riscv64", "ppc64le", "s390x",
		"kernel", "version", "my", "is", "it", "the",
		"on", "running", "now", "currently", "here", "use", "using",
		"i", "am",
		"mon", "tue", "wed", "thu", "fri", "sat", "sun",
		"jan", "feb", "mar", "apr", "may", "jun",
		"jul", "aug", "sep", "oct", "nov", "dec",
		"utc", "gmt",
	}
	for _, word := range words {
		if _, ok := benignKernelContextWords[word]; !ok {
			t.Errorf("%q is no longer allowed beside a release: every reply that includes it is now graded wrong", word)
			continue
		}
		if !kernelRule.Matches("6.12.3-gentoo " + word) {
			t.Errorf("Matches(%q) = false: a correct kernel version written beside an ordinary word was declined", "6.12.3-gentoo "+word)
		}
	}
	if kernelRule.Matches("6.12.3-gentoo buy-followers") {
		t.Error("an unlisted word was tolerated, so the acceptances above do not measure the allow-list")
	}
}

// The sentences and command lines the allow-list exists for, written the way people write them.
func TestAKernelVersionMayBeWrittenAsASentence(t *testing.T) {
	for _, text := range []string{
		"my kernel is 6.12.3 on aarch64",
		"i am currently running 6.12.3-gentoo",
		"6.12.3 smp preempt_dynamic",
		"the kernel version here is 6.12.3-gentoo",
		"i use 6.12.3-gentoo on riscv64",
		"uname -r 6.12.3-gentoo",
	} {
		if !kernelRule.Matches(text) {
			t.Errorf("Matches(%q) = false: a real Linux user's answer was graded wrong and charged one of three tries", text)
		}
	}
}

// A uname line ends in a timestamp that no allow-list can enumerate, so a build field marks the
// point after which the line is accepted as it stands. Without that, the ordinary uname answer is
// declined over its own clock and month names.
func TestAUnameLineIsAcceptedThroughItsBuildField(t *testing.T) {
	for _, text := range []string{
		"linux 6.12.3-gentoo #1 smp preempt_dynamic sat aug 22 14:56:02 aest 2026 x86_64 gnu/linux",
		"linux 6.12.3-gentoo #1 smp preempt mon sep 30 12:00:00 cst 2024 aarch64 android",
	} {
		if !kernelRule.Matches(text) {
			t.Errorf("Matches(%q) = false: a pasted uname line was declined over the timestamp it always carries", text)
		}
	}
	if kernelRule.Matches("linux 6.12.3-gentoo t.me/buyvpn") {
		t.Error("trailing text was accepted without a build field, so the acceptances above are not gated on uname shape")
	}
}

// The same clock and calendar numbers are tolerated only inside a uname line. Elsewhere a bare
// number is not part of an answer, and the allowance may not widen into padding around a version.
func TestBareClockNumbersAreToleratedOnlyInsideAUnameLine(t *testing.T) {
	unameLine := "linux 6.12.3-gentoo smp preempt_dynamic fri feb 2 09:25:10 utc 2024 x86_64"
	if !kernelRule.Matches(unameLine) {
		t.Errorf("Matches(%q) = false: a correct kernel version was declined because its timestamp carries bare numbers", unameLine)
	}
	if !kernelRule.Matches("6.12.3-gentoo smp") {
		t.Fatal("the same reply without the numbers was refused, so the refusals below prove nothing")
	}
	for _, text := range []string{
		"6.12.3-gentoo smp 2024",
		"6.12.3-gentoo 22",
	} {
		if kernelRule.Matches(text) {
			t.Errorf("Matches(%q) = true: bare digits became acceptable padding around a version outside a uname line", text)
		}
	}
}

// uname prints exactly one unvetted word, the hostname, and it stands immediately after "Linux".
// Any further free word would let a reply carry whatever it likes in front of the release.
func TestOnlyTheWordAfterLinuxMayBeAnUncheckedHostname(t *testing.T) {
	if !kernelRule.Matches("linux myhost 6.12.3-gentoo") {
		t.Fatal("the hostname slot itself was refused, so the refusals below prove nothing")
	}
	for _, text := range []string{
		"linux buy cheap followers 6.12.3 #1",
		"linux myhost buyvpn 6.12.3-gentoo",
	} {
		if kernelRule.Matches(text) {
			t.Errorf("Matches(%q) = true: a reply beginning with linux carried unvetted words in front of the release and was approved", text)
		}
	}
}

// People answer by pasting the command they ran together with its output. The echoed command line
// is stripped, so only the output is judged; leaving it in grades a correct answer wrong.
func TestAnEchoedCommandLineIsStrippedBeforeTheAnswerIsJudged(t *testing.T) {
	for _, test := range []struct {
		name string
		text string
	}{
		{name: "hostnamectl", text: "hostnamectl\nkernel: linux 6.12.3-gentoo"},
		{name: "hostnamectl behind a prompt", text: "$ hostnamectl\nkernel: linux 6.12.3-gentoo"},
		{name: "sudo uname", text: "sudo uname -a\nLinux gentoo 6.12.3-gentoo #1 SMP PREEMPT_DYNAMIC x86_64 GNU/Linux"},
		{name: "sudo uname behind a prompt", text: "root@box:~# sudo uname -a\nLinux gentoo 6.12.3-gentoo #1 SMP PREEMPT_DYNAMIC x86_64 GNU/Linux"},
		{name: "sudo cat /proc/version", text: "sudo cat /proc/version\nLinux version 6.12.3-gentoo (root@box) (gcc 14) #1 SMP"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !kernelRule.Matches(test.text) {
				t.Errorf("Matches(%q) = false: a correct kernel version was graded wrong because the pasted command line was left in", test.text)
			}
		})
	}
	if kernelRule.Matches("buy followers\nkernel: linux 6.12.3-gentoo") {
		t.Error("an ordinary first line was stripped too, so the acceptances above do not measure command-echo stripping")
	}
}
