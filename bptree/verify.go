package bptree

import (
	"bytes"
	"log"
)

import (
	"github.com/timtadh/fs2/consts"
	"github.com/timtadh/fs2/errors"
)

func (self *BpTree) Verify() (err error) {
	return self.verify(0, 0, self.meta.root, 0)
}

func (self *BpTree) verify(parent uint64, idx int, n, sibling uint64) (err error) {
	var flags consts.Flag
	err = self.bf.Do(n, 1, func(bytes []byte) error {
		flags = consts.AsFlag(bytes)
		return nil
	})
	if err != nil {
		return err
	}
	if flags&consts.INTERNAL != 0 {
		return self.internalVerify(parent, idx, n, sibling)
	} else if flags&consts.LEAF != 0 {
		return self.leafVerify(parent, idx, n, sibling)
	} else {
		return errors.Errorf("Unknown block type")
	}
}

func (self *BpTree) internalVerify(parent uint64, idx int, n, sibling uint64) (err error) {
	a := n
	return self.doInternal(a, func(n *internal) error {
		err := checkOrder(self.varchar, n)
		if err != nil {
			log.Println("error in internalVerify")
			log.Printf("out of order")
			log.Println("internal", a, n.Debug(self.varchar))
			return err
		}
		for i := 0; i < n.keyCount(); i++ {
			var sib uint64
			if i + 1 < n.keyCount() {
				sib = *n.ptr(i+1)
			} else if sibling != 0 {
				self.doInternal(sibling, func(sn *internal) error {
					sib = *sn.ptr(0)
					return nil
				})
			}
			err := self.verify(a, i, *n.ptr(i), sib)
			if err != nil {
				log.Println("------------------- internal -------------------")
				log.Println("error in internalVerify")
				log.Printf("could not verify node. failed kid %v", i)
				log.Println("n", a, n.Debug(self.varchar))
				if sibling != 0 {
					self.doInternal(sibling, func(n *internal) error {
						log.Println("sibing", sibling, n.Debug(self.varchar))
						return nil
					})
				}
				if parent != 0 {
					self.doInternal(parent, func(n *internal) error {
						log.Println("parent", parent, n.Debug(self.varchar))
						return nil
					})
				}
				log.Printf("n = %v, sibling = %v, parent = %v, parent idx = %v, i = %v, sib = %v", a, sibling, parent, idx, i, sib)
				return err
			}
		}
		return nil
	})
}

func (self *BpTree) leafVerify(parent uint64, idx int, n, sibling uint64) (err error) {
	a := n
	return self.doLeaf(a, func(n *leaf) error {
		if n.keyCount() == 0 {
			if parent != 0 {
				log.Println("warn, keyCount == 0", a, parent, sibling)
			}
			return nil
		}
		err := checkOrder(self.varchar, n)
		if err != nil {
			log.Println("error in leafVerify")
			log.Printf("out of order")
			log.Println("leaf", a, n.Debug(self.varchar))
			return err
		}
		if n.pure(self.varchar) {
			return self.pureVerify(parent, idx, a, sibling)
		}
		if n.meta.next != sibling {
			log.Println("error in leafVerify")
			log.Println("n.meta.next != sibling", n.meta.next, sibling)
			log.Println("leaf", a, n.Debug(self.varchar))
			return errors.Errorf("n.meta.next (%v) != sibling (%v)", n.meta.next, sibling)
		}
		return nil
	})
}

func (self *BpTree) pureVerify(parent uint64, idx int, n, sibling uint64) (err error) {
	a := n
	return self.doLeaf(a, func(n *leaf) error {
		e, err := self.endOfPureRun(a)
		if err != nil {
			log.Println("error in pureVerify")
			log.Println("end of pure run error")
			log.Println("leaf", a, n.Debug(self.varchar))
			return err
		}
		return self.doLeaf(e, func(m *leaf) (err error) {
			err = checkOrder(self.varchar, m)
			if err != nil {
				log.Println("error in pureVerify")
				log.Printf("e out of order")
				log.Println("leaf", e, n.Debug(self.varchar))
				return err
			}
			err = n.doKeyAt(self.varchar, 0, func(a_key_0 []byte) error {
				return m.doKeyAt(self.varchar, 0, func(e_key_0 []byte) error {
					if !bytes.Equal(a_key_0, e_key_0) {
						log.Println("a", a, n.Debug(self.varchar))
						log.Println("e", e, m.Debug(self.varchar))
						log.Println("went off of end of pure run")
						return errors.Errorf("End of pure run went off of pure run")
					}
					if m.meta.next == 0 {
						return nil
					}
					return self.doLeaf(m.meta.next, func(o *leaf) error {
						return o.doKeyAt(self.varchar, 0, func(o_key_0 []byte) error {
							if bytes.Equal(a_key_0, o_key_0) {
								log.Println("a", a, n.Debug(self.varchar))
								log.Println("e", e, m.Debug(self.varchar))
								log.Println("e.meta.next", m.meta.next, o.Debug(self.varchar))
								log.Println("did not find end of pure run")
								return errors.Errorf("did not find end of pure run")
							}
							return nil
						})
					})
				})
			})
			if err != nil {
				log.Println("error in pureVerify")
				return err
			}
			if m.meta.next != sibling {
				log.Println("error in pureVerify")
				log.Println("m.meta.next != sibling", m.meta.next, sibling)
				self.doLeaf(m.meta.next, func(o *leaf) error {
					log.Println("a", a, n.Debug(self.varchar))
					log.Println("e", e, m.Debug(self.varchar))
					log.Println("e.meta.next", m.meta.next, o.Debug(self.varchar))
					return nil
				})
				return errors.Errorf("m.meta.next (%v) != sibling (%v)", m.meta.next, sibling)
			}
			return nil
		})
	})
}