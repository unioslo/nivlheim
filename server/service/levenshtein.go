package main

//  Code taken from:
//  http://www.golangprograms.com/golang-program-for-implementation-of-levenshtein-distance.html

// LevenshteinDistance returns the minimum number of single-character edits
// (insertions, deletions or substitutions) required to change one string into the other.
// See also: https://en.wikipedia.org/wiki/Levenshtein_distance
func LevenshteinDistance(string1, string2 string) int {
	str1 := []rune(string1)
	str2 := []rune(string2)
	s1len := len(str1)
	s2len := len(str2)
	column := make([]int, len(str1)+1)
	for y := 1; y <= s1len; y++ {
		column[y] = y
	}
	for x := 1; x <= s2len; x++ {
		column[0] = x
		lastkey := x - 1
		for y := 1; y <= s1len; y++ {
			oldkey := column[y]
			var incr int
			if str1[y-1] != str2[x-1] {
				incr = 1
			}
			column[y] = minimum(column[y]+1, column[y-1]+1, lastkey+incr)
			lastkey = oldkey
		}
	}
	return column[s1len]
}

func minimum(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
	} else {
		if b < c {
			return b
		}
	}
	return c
}
