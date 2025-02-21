import { FunctionComponent, useEffect } from 'react'

import classNames from 'classnames'
import { formatDistance, format, parseISO } from 'date-fns'

import { Timestamp } from '@sourcegraph/branded/src/components/Timestamp'
import { TelemetryProps, TelemetryService } from '@sourcegraph/shared/src/telemetry/telemetryService'
import { Container, ErrorAlert, LoadingSpinner, PageHeader, H4, H3 } from '@sourcegraph/wildcard'

import { Collapsible } from '../../../../components/Collapsible'

import { useRankingSummary as defaultUseRankingSummary } from './backend'

import styles from './CodeIntelRankingPage.module.scss'

export interface CodeIntelRankingPageProps extends TelemetryProps {
    useRankingSummary?: typeof defaultUseRankingSummary
    telemetryService: TelemetryService
}

export const CodeIntelRankingPage: FunctionComponent<CodeIntelRankingPageProps> = ({
    useRankingSummary = defaultUseRankingSummary,
    telemetryService,
}) => {
    useEffect(() => telemetryService.logViewEvent('CodeIntelRankingPage'), [telemetryService])

    const { data, loading, error } = useRankingSummary({})

    if (loading && !data) {
        return <LoadingSpinner />
    }

    if (error) {
        return <ErrorAlert prefix="Failed to load code intelligence summary for repository" error={error} />
    }

    return (
        <>
            <PageHeader
                headingElement="h2"
                path={[
                    {
                        text: <>Ranking calculation history</>,
                    },
                ]}
                description="View the history of ranking calculation."
                className="mb-3"
            />

            <Container className="mb-3">
                {data &&
                    (data.rankingSummary.length === 0 ? (
                        <>No data.</>
                    ) : (
                        <>
                            <H3>Current ranking calculation ({data.rankingSummary[0].graphKey})</H3>

                            <div className="p-2">
                                <Summary
                                    key={data.rankingSummary[0].graphKey}
                                    summary={data.rankingSummary[0]}
                                    displayGraphKey={false}
                                />
                            </div>
                        </>
                    ))}
            </Container>

            {data && data.rankingSummary.length > 1 && (
                <Container>
                    <Collapsible title="Historic ranking calculations" titleAtStart={true} titleClassName="h3">
                        {data.rankingSummary.slice(1).map(summary => (
                            <Summary key={summary.graphKey} summary={summary} displayGraphKey={true} />
                        ))}
                    </Collapsible>
                </Container>
            )}
        </>
    )
}

interface Summary {
    graphKey: string
    pathMapperProgress: Progress
    referenceMapperProgress: Progress
    reducerProgress: Progress | null
}

interface Progress {
    startedAt: string
    completedAt: string | null
    processed: number
    total: number
}

interface SummaryProps {
    summary: Summary
    displayGraphKey: boolean
}

const Summary: FunctionComponent<SummaryProps> = ({ summary, displayGraphKey }) => (
    <div className="py-2">
        {displayGraphKey && <H4>Historic ranking calculation ({summary.graphKey})</H4>}

        <div className={displayGraphKey ? 'px-4' : ''}>
            <Progress title="Path Aggregation Process" progress={summary.pathMapperProgress} />

            <Progress
                title="Reference Aggregation Process"
                progress={summary.referenceMapperProgress}
                className="mt-4"
            />

            {summary.reducerProgress && (
                <Progress title="Reducing Process" progress={summary.reducerProgress} className="mt-4" />
            )}
        </div>
    </div>
)

interface ProgressProps {
    title: string
    progress: Progress
    className?: string
}

const Progress: FunctionComponent<ProgressProps> = ({ title, progress, className }) => (
    <div>
        <div className={classNames(styles.tableContainer, className)}>
            <H4 className="p-0 m-0">{title}</H4>
            <div className={styles.row}>
                <div>Queued records</div>
                <div>
                    {progress.total === 0 ? (
                        <>No records to process</>
                    ) : (
                        <>
                            {progress.processed} of {progress.total} records processed
                        </>
                    )}
                </div>
            </div>
            <div className={styles.row}>
                <div>Started</div>
                <div>
                    {format(parseISO(progress.startedAt), 'MMM d y h:mm:ss a')} (
                    <Timestamp date={progress.startedAt} />)
                </div>
            </div>
            <div className={styles.row}>
                <div>Completed</div>
                <div>
                    {progress.completedAt ? (
                        <>
                            {format(parseISO(progress.completedAt), 'MMM d y h:mm:ss a')} (
                            <Timestamp date={progress.completedAt} />)
                        </>
                    ) : (
                        '-'
                    )}
                </div>
            </div>
            <div className={styles.row}>
                <div>Duration</div>
                <div>
                    {progress.completedAt && (
                        <> Ran for {formatDistance(new Date(progress.completedAt), new Date(progress.startedAt))}</>
                    )}
                </div>
            </div>

            <div className={styles.row}>
                <div>Progress</div>
                <div>
                    {progress.processed === 0
                        ? 100
                        : Math.floor(((progress.processed / progress.total) * 100 * 100) / 100)}
                    %
                </div>
            </div>
        </div>
    </div>
)
