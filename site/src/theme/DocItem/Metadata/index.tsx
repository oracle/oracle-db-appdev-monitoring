import React, {type ReactNode} from 'react';
import {PageMetadata} from '@docusaurus/theme-common';
import {useDoc} from '@docusaurus/plugin-content-docs/client';
import docsSeo from '@site/src/data/docs-seo';

export default function DocItemMetadata(): ReactNode {
  const {metadata, frontMatter, assets} = useDoc();
  const pageSeo = docsSeo[metadata.id];

  return (
    <PageMetadata
      title={metadata.title}
      description={pageSeo?.description ?? metadata.description}
      keywords={pageSeo?.keywords ?? frontMatter.keywords}
      image={assets.image ?? frontMatter.image}
    />
  );
}
