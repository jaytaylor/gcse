package gcse

import (
	"time"

	"github.com/daviddengcn/sophie"
)

type CrawlingEntry struct {
	ScheduleTime time.Time
	// if gcse.CrawlerVersion is different from this value, etag is ignored
	Version int
	Etag    string
}

func (c *CrawlingEntry) WriteTo(w sophie.Writer) error {
	if err := sophie.Time(c.ScheduleTime).WriteTo(w); err != nil {
		return err
	}
	if err := sophie.VInt(c.Version).WriteTo(w); err != nil {
		return err
	}
	if err := sophie.String(c.Etag).WriteTo(w); err != nil {
		return err
	}
	return nil
}

func (c *CrawlingEntry) ReadFrom(r sophie.Reader, l int) error {
	if err := (*sophie.Time)(&c.ScheduleTime).ReadFrom(r, -1); err != nil {
		return err
	}
	if err := (*sophie.VInt)(&c.Version).ReadFrom(r, -1); err != nil {
		return err
	}
	if err := (*sophie.String)(&c.Etag).ReadFrom(r, -1); err != nil {
		return err
	}
	return nil
}
