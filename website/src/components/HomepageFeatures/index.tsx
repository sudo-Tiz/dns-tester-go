import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  icon: string;
  description: JSX.Element;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Multi-Protocol Support',
    icon: '🔐',
    description: (
      <>
        Test DNS protocols: <strong>Do53</strong> (UDP/TCP),
        <strong> DoT</strong>, <strong>DoH</strong>, and <strong>DoQ</strong>.
      </>
    ),
  },
  {
    title: 'Built with Go',
    icon: '⚡',
    description: (
      <>
        Query multiple DNS servers concurrently.
      </>
    ),
  },
  {
    title: 'Beta Software',
    icon: '🧪',
    description: (
      <>
        Under active development. <strong>Prometheus</strong> metrics,
        rate limiting, and async processing with Redis.
      </>
    ),
  },
  {
    title: 'REST API & CLI',
    icon: '🔌',
    description: (
      <>
        Use the <strong>CLI</strong> for quick tests or the <strong>REST API</strong>
        for automation.
      </>
    ),
  },
  {
    title: 'Easy Deployment',
    icon: '🐳',
    description: (
      <>
        Deploy with <strong>Docker Compose</strong>.
        Scale with worker pools.
      </>
    ),
  },
  {
    title: 'AI-Assisted Docs',
    icon: '🤖',
    description: (
      <>
        Documentation generated with help from <strong>AI agents</strong>.
        Report issues if you find errors.
      </>
    ),
  },
];

function Feature({title, icon, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center">
        <div className={styles.featureIcon}>{icon}</div>
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures(): JSX.Element {
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
