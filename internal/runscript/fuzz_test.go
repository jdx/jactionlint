package runscript

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzAnalyze is the `go test -fuzz` flavor of fuzz/runscript.go (which uses go-fuzz like the other targets).
func FuzzAnalyze(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "scripts", "*.sh"))
	for _, file := range files {
		if b, err := os.ReadFile(file); err == nil {
			f.Add(string(b))
		}
	}
	for _, seed := range []string{"", "${{", "${{ }}", "echo ${{ x }} >> $GITHUB_ENV", "cat <<EOF\n${{ a }}\nEOF", "a | b |& c", "$(($(echo)))", "{ ; }", "x=(a b)\n${x[@]}", "\\\n", "'", "\"${{ '}}' }}\""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		s, err := Analyze(src, "bash")
		if err != nil {
			return
		}
		if err := CheckInvariants(s); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
	})
}

func TestInvariantsOnTestdata(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "scripts", "*.sh"))
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		s, err := Analyze(string(b), "bash")
		if err != nil {
			continue
		}
		if err := CheckInvariants(s); err != nil {
			t.Errorf("%s: %v", file, err)
		}
	}
}

func BenchmarkAnalyze(b *testing.B) {
	files, _ := filepath.Glob(filepath.Join("testdata", "scripts", "real-*.sh"))
	var srcs []string
	total := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			b.Fatal(err)
		}
		srcs = append(srcs, string(data))
		total += len(data)
	}
	b.SetBytes(int64(total))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, src := range srcs {
			if _, err := Analyze(src, ""); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkAnalyzeSmall(b *testing.B) {
	const src = "echo \"VERSION=${{ steps.v.outputs.version }}\" >> \"$GITHUB_ENV\"\npip install requests==2.31.0\n"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Analyze(src, "bash"); err != nil {
			b.Fatal(err)
		}
	}
}
