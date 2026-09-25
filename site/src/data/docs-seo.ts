type DocSeoMetadata = {
  description: string;
  keywords: string[];
};

// Keys are docs IDs relative to site/docs (for example, "configuration/otlp").
// Titles remain sourced from each doc's front matter or its first heading.
const docsSeo: Record<string, DocSeoMetadata> = {
  'advanced/development': {
    description:
      'Learn how to build, test, and contribute to the Oracle AI Database Metrics Exporter.',
    keywords: ['Oracle AI Database Metrics Exporter', 'Go', 'development', 'contributing'],
  },
  'advanced/go-ora': {
    description:
      'Compile and run the exporter with the go-ora Oracle database driver without Oracle Instant Client or CGO.',
    keywords: ['go-ora', 'Oracle database driver', 'CGO', 'Oracle AI Database Metrics Exporter'],
  },
  'advanced/go-runtime': {
    description:
      'Control the exporter Go runtime resource limits for deployments with constrained memory or many exporter instances.',
    keywords: ['Go runtime', 'GOMEMLIMIT', 'GOMAXPROCS', 'memory limits'],
  },
  'advanced/older-versions': {
    description:
      'Configure the exporter to collect metrics from older Oracle Database releases.',
    keywords: ['Oracle Database compatibility', 'older database versions', 'metrics exporter'],
  },
  'advanced/txeventq': {
    description:
      'Monitor Oracle Transactional Event Queues (TxEventQ) with exporter metrics.',
    keywords: ['Transactional Event Queues', 'TxEventQ', 'Oracle Database monitoring'],
  },
  'configuration/alert-logs': {
    description:
      'Export Oracle Database alert logs in JSON for collection by a log ingestion tool.',
    keywords: ['Oracle alert log', 'JSON logs', 'database observability'],
  },
  'configuration/azure-vault': {
    description:
      'Configure Azure Key Vault so the exporter can retrieve Oracle Database credentials securely.',
    keywords: ['Azure Key Vault', 'Oracle credentials', 'secrets management'],
  },
  'configuration/config-file': {
    description:
      'Configure database connections, scraping, logging, and exporter behavior with YAML.',
    keywords: ['exporter configuration', 'YAML', 'Oracle Database', 'metrics scraping'],
  },
  'configuration/custom-metrics': {
    description:
      'Define Oracle Database queries and custom metrics in YAML or TOML for the exporter to collect.',
    keywords: ['custom metrics', 'Oracle SQL', 'YAML', 'TOML', 'Prometheus exporter'],
  },
  'configuration/hashicorp-vault': {
    description:
      'Configure HashiCorp Vault so the exporter can retrieve Oracle Database credentials securely.',
    keywords: ['HashiCorp Vault', 'Oracle credentials', 'secrets management'],
  },
  'configuration/multiple-databases': {
    description:
      'Configure one exporter instance to scrape metrics from multiple Oracle Database instances.',
    keywords: ['multiple databases', 'Oracle Database monitoring', 'Prometheus exporter'],
  },
  'configuration/oci-vault': {
    description:
      'Configure Oracle Cloud Infrastructure Vault so the exporter can retrieve database credentials securely.',
    keywords: ['OCI Vault', 'Oracle Cloud Infrastructure', 'database credentials'],
  },
  'configuration/oracle-wallet': {
    description:
      'Choose and configure plaintext, TLS, or Oracle Wallet authentication for database connections.',
    keywords: ['Oracle Wallet', 'mTLS', 'TLS', 'database authentication'],
  },
  'configuration/otlp': {
    description:
      'Publish Oracle Database metrics to an OpenTelemetry Protocol (OTLP) backend over gRPC alongside Prometheus scraping.',
    keywords: ['OTLP', 'OpenTelemetry', 'gRPC', 'Prometheus', 'Oracle Database metrics'],
  },
  'getting-started/basics': {
    description:
      'Install and start the Oracle AI Database Metrics Exporter, then connect it to an Oracle Database.',
    keywords: ['install metrics exporter', 'Oracle Database', 'getting started'],
  },
  'getting-started/default-metrics': {
    description:
      'Explore the built-in Oracle Database, process, and Go runtime metrics collected by the exporter.',
    keywords: ['default metrics', 'Oracle Database metrics', 'Prometheus', 'Go runtime'],
  },
  'getting-started/grafana-dashboards': {
    description:
      'Use the sample Grafana dashboards to visualize Oracle Database metrics from the exporter.',
    keywords: ['Grafana dashboards', 'Oracle Database monitoring', 'Prometheus metrics'],
  },
  'getting-started/kubernetes': {
    description:
      'Deploy the Oracle AI Database Metrics Exporter on Kubernetes using the provided manifests.',
    keywords: ['Kubernetes', 'Oracle Database exporter', 'deployment'],
  },
  intro: {
    description:
      'Monitor Oracle AI Database health, performance, and availability with Prometheus metrics and OpenTelemetry.',
    keywords: ['Oracle AI Database', 'metrics exporter', 'OpenTelemetry', 'Prometheus'],
  },
  'releases/builds': {
    description:
      'Find cross-platform Oracle AI Database Metrics Exporter builds and release download instructions.',
    keywords: ['exporter downloads', 'release builds', 'Linux', 'ARM64', 'AMD64'],
  },
  'releases/changelog': {
    description:
      'Review current and historical changes to the Oracle AI Database Metrics Exporter.',
    keywords: ['release notes', 'changelog', 'metrics exporter releases'],
  },
  'releases/roadmap': {
    description:
      'Review planned features and upcoming work for the Oracle AI Database Metrics Exporter.',
    keywords: ['roadmap', 'planned features', 'metrics exporter'],
  },
};

export default docsSeo;
