package inverterclock

import (
	"fmt"
	"time"
)

const (
	dateTimeAddress = 0x020C
	dateTimeWords   = 3
)

type WordAccessor interface {
	ReadHoldingWords(address, count uint16) ([]uint16, error)
	WriteHoldingWords(address uint16, values []uint16) error
}

type DateTime struct {
	Year      int    `json:"year"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	Hour      int    `json:"hour"`
	Minute    int    `json:"minute"`
	Second    int    `json:"second"`
	Formatted string `json:"formatted"`
	Valid     bool   `json:"valid"`
}

type Service struct {
	accessor WordAccessor
}

func New(accessor WordAccessor) *Service {
	return &Service{accessor: accessor}
}

func (s *Service) Read() (DateTime, error) {
	words, err := s.accessor.ReadHoldingWords(dateTimeAddress, dateTimeWords)
	if err != nil {
		return DateTime{}, err
	}
	return Decode(words)
}

func (s *Service) Set(value DateTime) (DateTime, error) {
	words, err := Encode(value)
	if err != nil {
		return DateTime{}, err
	}
	if err := s.accessor.WriteHoldingWords(dateTimeAddress, words); err != nil {
		return DateTime{}, err
	}
	return s.Read()
}

func Decode(words []uint16) (DateTime, error) {
	if len(words) < dateTimeWords {
		return DateTime{}, fmt.Errorf("inverter clock requires %d words, got %d", dateTimeWords, len(words))
	}

	rawYear := int(words[0] >> 8)
	year := 2000 + rawYear
	if rawYear >= 100 {
		// Some factory images expose the C tm_year representation.
		year = 1900 + rawYear
	}
	value := DateTime{
		Year:   year,
		Month:  int(words[0] & 0xFF),
		Day:    int(words[1] >> 8),
		Hour:   int(words[1] & 0xFF),
		Minute: int(words[2] >> 8),
		Second: int(words[2] & 0xFF),
	}
	if validate(value) == nil {
		value.Valid = true
		value.Formatted = fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", value.Year, value.Month, value.Day, value.Hour, value.Minute, value.Second)
	} else {
		value.Formatted = fmt.Sprintf("invalid raw clock (%d-%d-%d %d:%d:%d)", value.Year, value.Month, value.Day, value.Hour, value.Minute, value.Second)
	}
	return value, nil
}

func Encode(value DateTime) ([]uint16, error) {
	if err := validate(value); err != nil {
		return nil, err
	}
	return []uint16{
		uint16(value.Year-2000)<<8 | uint16(value.Month),
		uint16(value.Day)<<8 | uint16(value.Hour),
		uint16(value.Minute)<<8 | uint16(value.Second),
	}, nil
}

func ParseLocal(input string) (DateTime, error) {
	var parsed time.Time
	var err error
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04"} {
		parsed, err = time.ParseInLocation(layout, input, time.Local)
		if err == nil {
			return FromTime(parsed), nil
		}
	}
	return DateTime{}, fmt.Errorf("dateTime must use YYYY-MM-DDTHH:MM[:SS]")
}

func FromTime(value time.Time) DateTime {
	return DateTime{
		Year:   value.Year(),
		Month:  int(value.Month()),
		Day:    value.Day(),
		Hour:   value.Hour(),
		Minute: value.Minute(),
		Second: value.Second(),
	}
}

func validate(value DateTime) error {
	if value.Year < 2000 || value.Year > 2099 {
		return fmt.Errorf("inverter clock year must be between 2000 and 2099")
	}
	if value.Month < 1 || value.Month > 12 || value.Day < 1 || value.Day > 31 || value.Hour < 0 || value.Hour > 23 || value.Minute < 0 || value.Minute > 59 || value.Second < 0 || value.Second > 59 {
		return fmt.Errorf("invalid inverter date and time")
	}
	parsed := time.Date(value.Year, time.Month(value.Month), value.Day, value.Hour, value.Minute, value.Second, 0, time.UTC)
	if parsed.Year() != value.Year || int(parsed.Month()) != value.Month || parsed.Day() != value.Day {
		return fmt.Errorf("invalid inverter date and time")
	}
	return nil
}
