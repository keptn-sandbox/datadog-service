package utils

import logger "github.com/sirupsen/logrus"

type Formatter struct {
	Fields           logger.Fields
	BuiltinFormatter logger.Formatter
}

func (f *Formatter) Format(entry *logger.Entry) ([]byte, error) {
	for k, v := range f.Fields {
		entry.Data[k] = v
	}
	return f.BuiltinFormatter.Format(entry)
}
