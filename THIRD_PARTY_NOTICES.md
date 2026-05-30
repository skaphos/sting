# Third-Party Notices

sting ships and uses open source software from third-party projects.

The canonical notice artifacts live under [`third_party_licenses/`](third_party_licenses):

- `runtime/` and `runtime-report.csv` cover the dependencies included in the distributed `sting` binary.

## Regenerating notices

Regenerate the notice inventory and license texts whenever `go.mod` or `go.sum` changes:

```bash
go -C tools tool task notices
```

The task uses `github.com/google/go-licenses/v2@v2.0.1` to:

1. generate a CSV inventory of the applicable dependencies and detected licenses
2. copy the upstream license texts required for attribution into `third_party_licenses/`
