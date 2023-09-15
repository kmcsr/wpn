
package wpn

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/netip"
)

type IPRule struct {
	// An IP prefix, such as `192.168.0.1/24` or `::1/128`
	Prefix netip.Prefix
	// whether allow or not allow the prefix
	Not bool
}

func ParseIPRule(text string)(r *IPRule, err error){
	r = new(IPRule)
	if err = r.UnmarshalText(([]byte)(text)); err != nil {
		return
	}
	return
}

// Check if an IP match the rule.
// it will unmap the IP that passed in. for example, `127.0.0.1/8` contains `::ffff::127.1.2.3`
// note that it won't unmap the prefix, so `::ffff::127.0.0.1/104` doesn't contains `127.0.0.1`
func (r IPRule)Contains(ip netip.Addr)(ok bool){
	return r.Prefix.Contains(ip.Unmap()) != r.Not
}

// Parse an IP rule from text
// Empty bytes is not ok. It will return io.EOF if the text's length is zero
func (r *IPRule)UnmarshalText(buf []byte)(err error){
	if len(buf) == 0 {
		return io.EOF
	}
	if r.Not = buf[0] == '!'; r.Not {
		if len(buf) == 1 {
			return io.EOF
		}
		buf = buf[1:]
	}
	if bytes.LastIndexByte(buf, '/') < 0 {
		var addr netip.Addr
		if err = addr.UnmarshalText(buf); err != nil {
			return
		}
		if r.Prefix, err = addr.Prefix(addr.BitLen()); err != nil {
			return
		}
		return
	}
	if err = r.Prefix.UnmarshalText(buf); err != nil {
		return
	}
	return
}

func (r *IPRule)MarshalText()(buf []byte, err error){
	if r.Prefix.IsSingleIP() {
		if buf, err = r.Prefix.Addr().MarshalText(); err != nil {
			return
		}
	}else if buf, err = r.Prefix.MarshalText(); err != nil {
		return
	}
	if r.Not {
		buf = append(buf, 0)
		copy(buf[1:], buf)
		buf[0] = '!'
	}
	return
}


// IPRuleSet is a list of IPRule.
type IPRuleSet struct {
	rules []IPRule
}

// Create an empty rule set.
func NewIPRuleSet()(s *IPRuleSet){
	return &IPRuleSet{
		rules: make([]IPRule, 0, 10),
	}
}

// Create a rule set from a slice of rules.
// It's the invoker's responsibility to ensure the rules are only exists once in the slice.
func NewIPRuleSetFrom(rules []IPRule)(s *IPRuleSet){
	rules0 := make([]IPRule, len(rules))
	for i, v := range rules {
		rules0[i] = v
	}
	return &IPRuleSet{
		rules: rules0,
	}
}

// Read rule set from a text reader.
// `IPRuleSet.UnmarshalText` will be called
func ReadIPRuleSet(r io.Reader)(s *IPRuleSet, err error){
	s = NewIPRuleSet()
	var buf []byte
	if buf, err = io.ReadAll(r); err != nil {
		return
	}
	if err = s.UnmarshalText(buf); err != nil {
		return
	}
	return
}

// Check if an address is in the rule set or not.
// It will always return false if *IPRuleSet is nil.
// 
// Multiple include rules (Not == false) use `or (||)` operation.
// Multiple exclude rules (Not == true) use `and (&&)` operation.
// Exclude rules will always have the highest priority.
func (s *IPRuleSet)Contains(ip netip.Addr)(ok bool){
	if s == nil {
		return false
	}
	for _, r := range s.rules {
		if r.Not {
			if r.Contains(ip) {
				return false
			}
		}else if !ok {
			ok = r.Contains(ip)
		}
	}
	return
}

// Add a rule to the rule set.
// If the rule is already exists in the rule set, the method will do nothing and return false.
// Otherwise, the new rule will be append to the end, and return true.
// Rule conflict won't be detect
func (s *IPRuleSet)Add(r IPRule)(ok bool){
	for _, v := range s.rules {
		if v == r {
			return false
		}
	}
	s.rules = append(s.rules, r)
	return true
}

// Remove a rule from the rule set
func (s *IPRuleSet)Remove(r IPRule)(ok bool){
	for i, v := range s.rules {
		if v == r {
			copy(s.rules[i:], s.rules[i + 1:])
			s.rules = s.rules[:len(s.rules) - 1]
			return true
		}
	}
	return false
}

// Return the IP rule slice of the rule set.
// Operation on the slice it returns is not recommend and it's undefined.
// It will always return a nil slice if *IPRuleSet is nil.
func (s *IPRuleSet)Rules()([]IPRule){
	if s == nil {
		return nil
	}
	return s.rules
}

type RuleSetError struct {
	Line int
	Err error
}

func (e *RuleSetError)Unwrap()(error){
	return e.Err
}

func (e *RuleSetError)Error()(string){
	return fmt.Sprintf("%d: %s", e.Line, e.Err.Error())
}

// Parse a IPRuleSet file.
// each line will trim spaces before parse.
// empty line or line starts with hash sign ('#') will be ignored.
// During unmarshalling, the error lines will be skipped,
//  the errors will be returned after wrapped with `RuleSetError` and `errors.Join`.
func (s *IPRuleSet)UnmarshalText(buf []byte)(error){
	l := s.rules
	if len(l) != 0 {
		l = make([]IPRule, 0, 10)
	}

	var errs []error
	sc := bufio.NewScanner(bytes.NewReader(buf))
	for i := 0; sc.Scan(); i++ {
		bts := bytes.TrimSpace(sc.Bytes())
		if len(bts) > 0 && bts[0] != '#' {
			var r IPRule
			if err := r.UnmarshalText(bts); err != nil {
				errs = append(errs, &RuleSetError{i, err})
				continue
			}
			l = append(l, r)
		}
	}

	s.rules = l
	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *IPRuleSet)MarshalText()(_ []byte, err error){
	var buf bytes.Buffer
	for _, r := range s.rules {
		var b []byte
		if b, err = r.MarshalText(); err != nil {
			return
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
