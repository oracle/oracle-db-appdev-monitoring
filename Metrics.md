## Metrics Exporter 

###Metrics export entries contain the following elements:

- `context`: Overall context of log entry
- `metricsdesc`: Description of metric that maps to values obtained via `request`
- `labels`: Field used to create labels for metric that maps to values obtained via `request`
- `request`: The query used to retrieve metric information. 

The following are example entries both without and with labels.

```toml
[[metric]]
context = "context_no_label"
request = "SELECT 1 as value_1, 2 as value_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }

[[metric]]
context = "context_with_labels"
labels = [ "label_1", "label_2" ]
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }
```