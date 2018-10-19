### Tiny consul wrapper

This wrapper works with structures and allows to unmarshal consul configuration right into your structure.

If key is not in consul, wrappers adds it and sets default value from structures field tags.

### Environment variables

##### GROUP_NAME
used for setting up global folder for keys. All keys will be accessed by path like GROUP_NAME/key
