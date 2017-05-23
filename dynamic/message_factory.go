package dynamic

import (
	"reflect"
	"sync"

	"github.com/golang/protobuf/proto"

	"github.com/jhump/protoreflect/desc"
)

// MessageFactory can be used to create new empty message objects.
type MessageFactory struct {
	er  *ExtensionRegistry
	ktr *KnownTypeRegistry
}

// NewMessageFactoryWithExtensionRegistry creates a new message factory where any
// dynamic messages produced will use the given extension registry to recognize and
// parse extension fields.
func NewMessageFactoryWithExtensionRegistry(er *ExtensionRegistry) *MessageFactory {
	return NewMessageFactoryWithRegistries(er, nil)
}

// NewMessageFactoryWithKnownTypeRegistry creates a new message factory where the
// known types, per the given registry, will be returned as normal protobuf messages
// (e.g. generated structs, instead of dynamic messages).
func NewMessageFactoryWithKnownTypeRegistry(ktr *KnownTypeRegistry) *MessageFactory {
	return NewMessageFactoryWithRegistries(nil, ktr)
}

// NewMessageFactoryWithDefaults creates a new message factory where all "default" types
// (those for which protoc-generated code is statically linked into the Go program) are
// known types. Is any dynamic messages are produced, they will recognize and parse all
// "default" extension fields. This is the equivalent of:
//   NewMessageFactoryWithRegistries(
//       NewExtensionRegistryWithDefaults(),
//       NewKnownTypeRegistryWithDefaults())
func NewMessageFactoryWithDefaults() *MessageFactory {
	return NewMessageFactoryWithRegistries(NewExtensionRegistryWithDefaults(), NewKnownTypeRegistryWithDefaults())
}

// NewMessageFactoryWithRegistries creates a new message factory with the given extension
// and known type registries.
func NewMessageFactoryWithRegistries(er *ExtensionRegistry, ktr *KnownTypeRegistry) *MessageFactory {
	return &MessageFactory{
		er:  er,
		ktr: ktr,
	}
}

// NewMessage creates a new empty message that corresponds to the given descriptor.
// If the given descriptor describes a "known type" then that type is instantiated.
// Otherwise, an empty dynamic message is returned.
func (f *MessageFactory) NewMessage(md *desc.MessageDescriptor) proto.Message {
	if f == nil {
		return NewMessage(md)
	}
	if m := f.ktr.CreateIfKnown(md.GetFullyQualifiedName()); m != nil {
		return m
	}
	return newMessageWithMessageFactory(md, f)
}

type wkt interface {
	XXX_WellKnownType() string
}

var typeOfWkt = reflect.TypeOf((*wkt)(nil)).Elem()

// KnownTypeRegistry is a registry of known message types, as identified by their
// fully-qualified name. A known message type is one for which a protoc-generated
// struct exists, so a dynamic message is not necessary to represent it. A
// MessageFactory uses a KnownTypeRegistry to decide whether to create a generated
// struct or a dynamic message. The zero-value registry (including the behavior of
// a nil pointer) only knows about the "well-known types" in protobuf. These
// include only the wrapper types and a handful of other special types like Any,
// Duration, and Timestamp.
type KnownTypeRegistry struct {
	excludeWkt     bool
	includeDefault bool
	mu             sync.RWMutex
	types          map[string]reflect.Type
}

// NewKnownTypeRegistryWithDefaults creates a new registry that knows about all
// "default" types (those for which protoc-generated code is statically linked
// into the Go program).
func NewKnownTypeRegistryWithDefaults() *KnownTypeRegistry {
	return &KnownTypeRegistry{includeDefault: true}
}

// NewKnownTypeRegistryWithoutWellKnownTypes creates a new registry that does *not*
// include the "well-known types" in protobuf. So even well-known types would be
// represented by a dynamic message.
func NewKnownTypeRegistryWithoutWellKnownTypes() *KnownTypeRegistry {
	return &KnownTypeRegistry{excludeWkt: true}
}

// AddKnownType adds the types of the given messages as known types.
func (r *KnownTypeRegistry) AddKnownType(kts ...proto.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.types == nil {
		r.types = map[string]reflect.Type{}
	}
	for _, kt := range kts {
		r.types[proto.MessageName(kt)] = reflect.TypeOf(kt)
	}
}

// CreateIfKnown will construct an instance of the given message if it is a known type.
// If the given name is unknown, nil is returned.
func (r *KnownTypeRegistry) CreateIfKnown(messageName string) proto.Message {
	var msgType reflect.Type
	if r == nil {
		// a nil registry behaves the same as zero value instance: only know of well-known types
		t := proto.MessageType(messageName)
		if t != nil && t.Implements(typeOfWkt) {
			msgType = t
		}
	} else {
		if r.includeDefault {
			msgType = proto.MessageType(messageName)
		} else if !r.excludeWkt {
			t := proto.MessageType(messageName)
			if t != nil && t.Implements(typeOfWkt) {
				msgType = t
			}
		}
		if msgType == nil {
			r.mu.RLock()
			msgType = r.types[messageName]
			r.mu.RUnlock()
		}
	}

	if msgType == nil {
		return nil
	}

	if msgType.Kind() == reflect.Ptr {
		return reflect.New(msgType.Elem()).Interface().(proto.Message)
	} else {
		return reflect.New(msgType).Elem().Interface().(proto.Message)
	}
}