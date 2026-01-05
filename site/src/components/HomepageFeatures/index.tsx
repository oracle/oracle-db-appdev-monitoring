import type {ReactNode} from 'react';
import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  Svg: React.ComponentType<React.ComponentProps<'svg'>>;
  description: ReactNode;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Monitor From Anywhere',
    Svg: require('@site/static/img/logo.svg').default,
    description: (
      <>
        Run the Oracle AI Database Metrics Exporter as a local binary, container, or in Kubernetes.
          Use pre-built AMD64 and ARM64 images to easily get started.
      </>
    ),
  },
  {
    title: 'Extensible Database Metrics',
    Svg: require('@site/static/img/logo.svg').default,
    description: (
      <>
        Use the default, include database metrics or define custom metrics with plain SQL queries in simple <code>YAML</code> or <code>TOML</code> files.
      </>
    ),
  },
  {
    title: 'Multiple Databases? No Problem',
    Svg: require('@site/static/img/logo.svg').default,
    description: (
      <>
        Easily monitor one or more databases with a single exporter.
          One of your databases down or under maintenance? You'll still receive metrics from the others.
      </>
    ),
  },
];

function Feature({title, Svg, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center">
        <Svg className={styles.featureSvg} role="img" />
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
